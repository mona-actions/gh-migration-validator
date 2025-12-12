package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// isSafeSymlinkTarget checks if a symlink target will remain within the destPath after resolution.
func isSafeSymlinkTarget(linkname, destPath string) bool {
	// Refuse absolute targets
	if filepath.IsAbs(linkname) {
		return false
	}
	targetPath := filepath.Join(destPath, linkname)
	realTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		// If cannot resolve, be conservative: refuse
		return false
	}
	cleanDestPath := filepath.Clean(destPath)
	relpath, err := filepath.Rel(cleanDestPath, realTarget)
	return err == nil && !strings.HasPrefix(filepath.Clean(relpath), "..")
}

// ExtractTarGz extracts a .tar.gz file to the specified destination directory
func ExtractTarGz(srcPath, destPath string) error {
	// Open the source file
	file, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open archive file: %v", err)
	}
	defer file.Close()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %v", err)
	}

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %v", err)
		}

		// Skip problematic paths
		if header.Name == "" || header.Name == "." || header.Name == "./" {
			continue
		}

		// Construct the full path for the file
		fullPath := filepath.Join(destPath, header.Name)

		// Security check: ensure the file path is within the destination directory
		cleanDestPath := filepath.Clean(destPath)
		cleanFullPath := filepath.Clean(fullPath)

		// Check if the clean full path is within the destination directory
		// Also prevent files from overwriting the destination directory itself
		if !strings.HasPrefix(cleanFullPath, cleanDestPath+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s (resolved to %s, outside %s)", header.Name, cleanFullPath, cleanDestPath)
		}

		// Handle different file types
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(fullPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", fullPath, err)
			}

		case tar.TypeReg:
			// Create file
			if err := extractFile(tarReader, fullPath, header.Mode); err != nil {
				return fmt.Errorf("failed to extract file %s: %v", fullPath, err)
			}

		case tar.TypeSymlink:
			// Securely create symbolic link after validating target
			if !isSafeSymlinkTarget(header.Linkname, destPath) {
				return fmt.Errorf("symlink target escapes extraction directory: %s -> %s", fullPath, header.Linkname)
			}
			if err := os.Symlink(header.Linkname, fullPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %v", fullPath, err)
			}

		default:
			// Skip other file types (block devices, character devices, etc.)
			log.Printf("Skipping unsupported file type for %s (type: %d)\n", header.Name, header.Typeflag)
		}
	}

	return nil
}

// extractFile extracts a single regular file from the tar archive
func extractFile(tarReader *tar.Reader, destPath string, mode int64) error {
	// Ensure the parent directory exists
	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %v", err)
	}

	// Create the file
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Copy the file contents
	_, err = io.Copy(file, tarReader)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %v", err)
	}

	// Set file permissions
	if err := os.Chmod(destPath, os.FileMode(mode)); err != nil {
		return fmt.Errorf("failed to set file permissions: %v", err)
	}

	return nil
}

// GetArchiveDestination generates a destination directory name based on the archive filename
func GetArchiveDestination(archivePath string) string {
	baseName := filepath.Base(archivePath)
	// Remove .tar.gz extension
	baseName = strings.TrimSuffix(baseName, ".tar.gz")
	// Remove .tgz extension
	baseName = strings.TrimSuffix(baseName, ".tgz")

	// Return the directory path in the same location as the archive
	archiveDir := filepath.Dir(archivePath)
	return filepath.Join(archiveDir, baseName)
}
