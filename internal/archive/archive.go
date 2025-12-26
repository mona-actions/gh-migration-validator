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

// isSafeSymlinkTarget checks if both the symlink and its target will remain within destPath after resolution.
func isSafeSymlinkTarget(symlinkPath, linkname, destPath string) (bool, error) {
	// Refuse absolute symlink targets
	if filepath.IsAbs(linkname) {
		return false, fmt.Errorf("absolute symlink target not allowed: %s", linkname)
	}

	// 1. Validate the symlink location itself resolves within destPath
	resolvedSymlinkPath, err := filepath.EvalSymlinks(filepath.Dir(symlinkPath))
	if err != nil {
		// If parent directory doesn't exist yet or can't be resolved, use the path as-is
		resolvedSymlinkPath = filepath.Dir(symlinkPath)
	}
	resolvedSymlinkPath = filepath.Join(resolvedSymlinkPath, filepath.Base(symlinkPath))

	cleanDestPath := filepath.Clean(destPath)
	relToRoot, err := filepath.Rel(cleanDestPath, filepath.Clean(resolvedSymlinkPath))
	if err != nil || strings.HasPrefix(filepath.Clean(relToRoot), "..") {
		return false, fmt.Errorf("symlink escapes destination: %s", symlinkPath)
	}

	// 2. Validate the symlink target (interpreted relative to symlink's parent) resolves within destPath
	symlinkParent := filepath.Dir(resolvedSymlinkPath)
	targetPath := filepath.Join(symlinkParent, linkname)

	// Try to resolve the target path; if it doesn't exist yet, clean it
	resolvedTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		// Target doesn't exist yet, use cleaned path
		resolvedTarget = filepath.Clean(targetPath)
	}

	relTarget, err := filepath.Rel(cleanDestPath, resolvedTarget)
	if err != nil || strings.HasPrefix(filepath.Clean(relTarget), "..") {
		return false, fmt.Errorf("symlink target escapes destination: %s -> %s", symlinkPath, linkname)
	}

	return true, nil
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

	// Resolve destPath to handle any symlinks in the path itself
	// This ensures consistent path comparisons throughout extraction
	resolvedDestPath, err := filepath.EvalSymlinks(destPath)
	if err != nil {
		// If we can't resolve, use the original (shouldn't happen after MkdirAll)
		resolvedDestPath = destPath
	}
	cleanDestPath := filepath.Clean(resolvedDestPath)

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

		// Reject absolute paths in archives
		if filepath.IsAbs(header.Name) {
			return fmt.Errorf("invalid file path: %s (absolute paths not allowed)", header.Name)
		}

		// Construct the full path for the file using the resolved destination path
		fullPath := filepath.Join(resolvedDestPath, header.Name)

		// Security check: ensure the file path is within the destination directory
		cleanFullPath := filepath.Clean(fullPath)

		// Check if the clean full path is within the destination directory
		// Also prevent files from overwriting the destination directory itself
		if !strings.HasPrefix(cleanFullPath, cleanDestPath+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s (resolved to %s, outside %s)", header.Name, cleanFullPath, cleanDestPath)
		}

		// Additional security check: resolve symlinks in the path to prevent escapes via existing symlinks
		parentDir := filepath.Dir(fullPath)
		resolvedParent, err := filepath.EvalSymlinks(parentDir)
		if err != nil {
			// Parent doesn't exist yet - use the cleaned path for validation
			resolvedParent = filepath.Clean(parentDir)
		}
		resolvedFullPath := filepath.Join(resolvedParent, filepath.Base(fullPath))

		// Make sure the resolved path is still within destPath
		cleanResolvedPath := filepath.Clean(resolvedFullPath)
		if !strings.HasPrefix(cleanResolvedPath, cleanDestPath+string(os.PathSeparator)) &&
			cleanResolvedPath != cleanDestPath {
			return fmt.Errorf("path escapes extraction directory after symlink resolution: %s", header.Name)
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
			// Securely create symbolic link after validating both symlink location and target
			safe, err := isSafeSymlinkTarget(fullPath, header.Linkname, resolvedDestPath)
			if !safe || err != nil {
				if err != nil {
					return err
				}
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
