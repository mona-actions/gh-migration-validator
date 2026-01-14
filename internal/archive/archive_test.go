package archive

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetArchiveDestination(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tar.gz extension",
			input:    "/path/to/archive.tar.gz",
			expected: "/path/to/archive",
		},
		{
			name:     "tgz extension",
			input:    "/path/to/archive.tgz",
			expected: "/path/to/archive",
		},
		{
			name:     "nested path",
			input:    "/deep/nested/path/migration-123.tar.gz",
			expected: "/deep/nested/path/migration-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetArchiveDestination(tt.input)
			if result != tt.expected {
				t.Errorf("GetArchiveDestination(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractTarGz_ValidArchive(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "test.tar.gz")
	extractPath := filepath.Join(tempDir, "extract")

	// Create a valid test archive
	err := createTestArchive(archivePath, []testFile{
		{name: "file1.txt", content: "Hello World", fileType: tar.TypeReg},
		{name: "dir1/", content: "", fileType: tar.TypeDir},
		{name: "dir1/file2.txt", content: "Nested file", fileType: tar.TypeReg},
	})
	if err != nil {
		t.Fatalf("Failed to create test archive: %v", err)
	}

	// Extract the archive
	err = ExtractTarGz(archivePath, extractPath)
	if err != nil {
		t.Fatalf("ExtractTarGz failed: %v", err)
	}

	// Verify extracted files
	tests := []struct {
		path    string
		content string
		isDir   bool
	}{
		{path: "file1.txt", content: "Hello World", isDir: false},
		{path: "dir1", content: "", isDir: true},
		{path: "dir1/file2.txt", content: "Nested file", isDir: false},
	}

	for _, tt := range tests {
		fullPath := filepath.Join(extractPath, tt.path)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Errorf("File %s should exist: %v", tt.path, err)
			continue
		}

		if tt.isDir != info.IsDir() {
			t.Errorf("File %s: isDir = %v, want %v", tt.path, info.IsDir(), tt.isDir)
		}

		if !tt.isDir {
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Errorf("Failed to read %s: %v", tt.path, err)
				continue
			}
			if string(content) != tt.content {
				t.Errorf("File %s: content = %q, want %q", tt.path, string(content), tt.content)
			}
		}
	}
}

func TestExtractTarGz_PathTraversal(t *testing.T) {
	tests := []struct {
		name          string
		fileName      string
		shouldError   bool
		errorContains string
	}{
		{
			name:          "directory traversal with dot-dot",
			fileName:      "../../../etc/passwd",
			shouldError:   true,
			errorContains: "outside",
		},
		{
			name:          "absolute path",
			fileName:      "/etc/passwd",
			shouldError:   true,
			errorContains: "absolute paths not allowed",
		},
		{
			name:        "valid nested path",
			fileName:    "safe/nested/file.txt",
			shouldError: false,
		},
		{
			name:          "directory traversal in middle",
			fileName:      "dir/../../../etc/passwd",
			shouldError:   true,
			errorContains: "outside",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			archivePath := filepath.Join(tempDir, "test.tar.gz")
			extractPath := filepath.Join(tempDir, "extract")

			// Create archive with the test file
			err := createTestArchive(archivePath, []testFile{
				{name: tt.fileName, content: "content", fileType: tar.TypeReg},
			})
			if err != nil {
				t.Fatalf("Failed to create test archive: %v", err)
			}

			// Try to extract
			err = ExtractTarGz(archivePath, extractPath)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error %q should contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestExtractTarGz_MaliciousSymlinks(t *testing.T) {
	tests := []struct {
		name          string
		linkname      string
		target        string
		shouldError   bool
		errorContains string
	}{
		{
			name:          "absolute symlink target",
			linkname:      "evil-link",
			target:        "/etc/passwd",
			shouldError:   true,
			errorContains: "absolute symlink target not allowed",
		},
		{
			name:          "symlink escaping via dot-dot",
			linkname:      "evil-link",
			target:        "../../../etc/passwd",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:          "symlink in subdir escaping",
			linkname:      "subdir/evil-link",
			target:        "../../../../etc/passwd",
			shouldError:   true,
			errorContains: "escapes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			archivePath := filepath.Join(tempDir, "test.tar.gz")
			extractPath := filepath.Join(tempDir, "extract")

			// Create archive with malicious symlink
			f, err := os.Create(archivePath)
			if err != nil {
				t.Fatalf("Failed to create archive: %v", err)
			}
			defer f.Close()

			gw := gzip.NewWriter(f)
			defer gw.Close()
			tw := tar.NewWriter(gw)
			defer tw.Close()

			// Create all parent directories for the symlink
			parts := strings.Split(tt.linkname, "/")
			for i := 0; i < len(parts)-1; i++ {
				dirPath := strings.Join(parts[:i+1], "/")
				if err := tw.WriteHeader(&tar.Header{
					Name:     dirPath + "/",
					Mode:     0755,
					Typeflag: tar.TypeDir,
				}); err != nil {
					t.Fatalf("Failed to write directory header: %v", err)
				}
			}

			// Create the symlink
			if err := tw.WriteHeader(&tar.Header{
				Name:     tt.linkname,
				Linkname: tt.target,
				Typeflag: tar.TypeSymlink,
			}); err != nil {
				t.Fatalf("Failed to write symlink header: %v", err)
			}

			tw.Close()
			gw.Close()
			f.Close()

			// Try to extract
			err = ExtractTarGz(archivePath, extractPath)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error %q should contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestExtractTarGz_MaliciousSymlinks_Extended(t *testing.T) {
	tests := []struct {
		name          string
		linkname      string
		target        string
		shouldError   bool
		errorContains string
	}{
		{
			name:          "symlink name escapes directory",
			linkname:      "../evil-link",
			target:        "anything.txt",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:          "symlink name with multiple dot-dots",
			linkname:      "../../evil-link",
			target:        "target.txt",
			shouldError:   true,
			errorContains: "outside",
		},
		{
			name:          "symlink target with single dot-dot at boundary",
			linkname:      "link.txt",
			target:        "../outside.txt",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:          "symlink target with two dot-dots at boundary",
			linkname:      "subdir/link.txt",
			target:        "../../outside.txt",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:          "deeply nested symlink escaping by one level",
			linkname:      "a/b/c/d/link.txt",
			target:        "../../../../../outside.txt",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:        "deeply nested symlink just at boundary",
			linkname:    "a/b/c/link.txt",
			target:      "../../../safe.txt",
			shouldError: false,
		},
		{
			name:          "symlink with mixed path separators attempting escape",
			linkname:      "dir/link.txt",
			target:        "../../../etc/passwd",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:          "symlink in root with single dot-dot escape",
			linkname:      "link.txt",
			target:        "../outside",
			shouldError:   true,
			errorContains: "escapes",
		},
		{
			name:        "symlink with complex relative path staying inside",
			linkname:    "a/b/link.txt",
			target:      "../c/target.txt",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			archivePath := filepath.Join(tempDir, "test.tar.gz")
			extractPath := filepath.Join(tempDir, "extract")

			// Create archive with the symlink
			f, err := os.Create(archivePath)
			if err != nil {
				t.Fatalf("Failed to create archive: %v", err)
			}
			defer f.Close()

			gw := gzip.NewWriter(f)
			defer gw.Close()
			tw := tar.NewWriter(gw)
			defer tw.Close()

			// Create all parent directories for the symlink
			parts := strings.Split(tt.linkname, "/")
			for i := 0; i < len(parts)-1; i++ {
				dirPath := strings.Join(parts[:i+1], "/")
				if err := tw.WriteHeader(&tar.Header{
					Name:     dirPath + "/",
					Mode:     0755,
					Typeflag: tar.TypeDir,
				}); err != nil {
					t.Fatalf("Failed to write directory header: %v", err)
				}
			}

			// Create the symlink
			if err := tw.WriteHeader(&tar.Header{
				Name:     tt.linkname,
				Linkname: tt.target,
				Typeflag: tar.TypeSymlink,
			}); err != nil {
				t.Fatalf("Failed to write symlink header: %v", err)
			}

			tw.Close()
			gw.Close()
			f.Close()

			// Try to extract
			err = ExtractTarGz(archivePath, extractPath)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error %q should contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestExtractTarGz_SafeSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "test.tar.gz")
	extractPath := filepath.Join(tempDir, "extract")

	// Create archive with a file and a safe symlink to it
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add a regular file
	if err := tw.WriteHeader(&tar.Header{
		Name:     "target.txt",
		Mode:     0644,
		Size:     7,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}
	if _, err := tw.Write([]byte("content")); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}

	// Add a safe relative symlink
	if err := tw.WriteHeader(&tar.Header{
		Name:     "link.txt",
		Linkname: "target.txt",
		Typeflag: tar.TypeSymlink,
	}); err != nil {
		t.Fatalf("Failed to write symlink header: %v", err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	// Extract the archive
	err = ExtractTarGz(archivePath, extractPath)
	if err != nil {
		t.Fatalf("ExtractTarGz failed on safe symlink: %v", err)
	}

	// Verify the symlink was created
	linkPath := filepath.Join(extractPath, "link.txt")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Symlink should exist: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symlink but got regular file")
	}

	// Verify symlink points to correct target
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if target != "target.txt" {
		t.Errorf("Symlink target = %q, want %q", target, "target.txt")
	}
}

// Helper types and functions

type testFile struct {
	name     string
	content  string
	fileType byte
}

func createTestArchive(path string, files []testFile) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, file := range files {
		if file.fileType == tar.TypeDir {
			header := &tar.Header{
				Name:     file.name,
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
		} else {
			header := &tar.Header{
				Name:     file.name,
				Mode:     0644,
				Size:     int64(len(file.content)),
				Typeflag: tar.TypeReg,
			}
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if _, err := io.WriteString(tw, file.content); err != nil {
				return err
			}
		}
	}

	return nil
}

func createTestArchiveWithSymlink(path, linkname, target string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	header := &tar.Header{
		Name:     linkname,
		Linkname: target,
		Typeflag: tar.TypeSymlink,
	}
	return tw.WriteHeader(header)
}
