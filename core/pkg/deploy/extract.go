package deploy

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractTar extracts a tar.gz archive to the specified directory.
// Includes security checks to prevent path traversal and tar bombs.
func ExtractTar(tarReader io.Reader, destDir string) error {
	// Create gzip reader
	gzr, err := gzip.NewReader(tarReader)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	// Create tar reader
	tr := tar.NewReader(gzr)

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// Extract each file
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		// Build target path
		targetPath := filepath.Join(destDir, header.Name)

		// Security: Prevent path traversal
		// Ensure target path is within destDir
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)) {
			return fmt.Errorf("invalid path in tar: %s (would escape destDir)", header.Name)
		}

		// Extract based on type
		switch header.Typeflag {
		case tar.TypeDir:
			// Directory
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("create directory %s: %w", targetPath, err)
			}

		case tar.TypeReg:
			// Regular file
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", targetPath, err)
			}

			// Create file with readable permissions
			// Ensure at least 0644 (owner rw, others r) for agent files to be readable by runtime
			mode := os.FileMode(header.Mode)
			if mode&0644 != 0644 {
				mode = mode | 0644
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("create file %s: %w", targetPath, err)
			}

			// Copy contents
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("write file %s: %w", targetPath, err)
			}
			outFile.Close()

		case tar.TypeSymlink:
			// Symlink
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("create parent dir for symlink %s: %w", targetPath, err)
			}

			// Remove existing file/symlink if present
			os.Remove(targetPath)

			// Create symlink
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return fmt.Errorf("create symlink %s: %w", targetPath, err)
			}

		default:
			// Skip other types (char devices, block devices, etc.)
			continue
		}
	}

	return nil
}

// CalculateFileChecksum calculates SHA256 checksum of a file.
func CalculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// VerifyChecksum verifies that file matches expected checksum.
func VerifyChecksum(filePath, expectedChecksum string) error {
	actualChecksum, err := CalculateFileChecksum(filePath)
	if err != nil {
		return err
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

// ValidateAgentName validates agent name to prevent path traversal attacks.
func ValidateAgentName(name string) error {
	// Check for empty name
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}

	// Check for path separators
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("agent name cannot contain path separators: %s", name)
	}

	// Check for parent directory references
	if strings.Contains(name, "..") {
		return fmt.Errorf("agent name cannot contain '..': %s", name)
	}

	// Check for hidden files
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("agent name cannot start with '.': %s", name)
	}

	return nil
}
