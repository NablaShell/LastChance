// storage/paths.go
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SafeFSOps provides secure file system operations,
// preventing Path Traversal (CWE-22) via sandboxing.
type SafeFSOps struct {
	root string
}

// NewSafeFSOps создаёт песочницу с указанным корневым путём.
// Все операции будут ограничены этой директорией.
func NewSafeFSOps(root string) (*SafeFSOps, error) {
	cleanRoot := filepath.Clean(root)

	// Checking existence and rights
	info, err := os.Stat(cleanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("storage root does not exist: %s", cleanRoot)
		}
		return nil, fmt.Errorf("cannot access storage root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("storage root is not a directory: %s", cleanRoot)
	}

	return &SafeFSOps{root: cleanRoot}, nil
}

// ResolvePath clears and checks that the path is inside root.
func (s *SafeFSOps) ResolvePath(userPath string) (string, error) {
	// Step 1: Path cleaning (removes ../, ./, extra slashes)
	cleanPath := filepath.Clean(userPath)

	// Step 2: Making it absolute relative root
	absPath := filepath.Join(s.root, cleanPath)

	// Step 3: Let's check that we haven't gone overboard root
	rel, err := filepath.Rel(s.root, absPath)
	if err != nil {
		return "", fmt.Errorf("path resolution failed: %w", err)
	}

	// We prohibit exit through "../../../"
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal detected: %s", userPath)
	}

	return absPath, nil
}

// SafeOpenFile opens the file only inside sandbox.
func (s *SafeFSOps) SafeOpenFile(userPath string) (*os.File, error) {
	safePath, err := s.ResolvePath(userPath)
	if err != nil {
		return nil, err
	}
	// #nosec G304 — safePath has already been checked ResolvePath
	file, err := os.Open(safePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", safePath, err)
	}

	return file, nil
}

// SafeWriteFile writes the file only inside sandbox.
func (s *SafeFSOps) SafeWriteFile(userPath string, data []byte, perm os.FileMode) error {
	safePath, err := s.ResolvePath(userPath)
	if err != nil {
		return err
	}

	// Make sure the parent directory exists
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}

	return os.WriteFile(safePath, data, perm)
}

// SafeReadFile reads the file only inside sandbox.
func (s *SafeFSOps) SafeReadFile(userPath string) ([]byte, error) {
	safePath, err := s.ResolvePath(userPath)
	if err != nil {
		return nil, err
	}
	// #nosec G304 — safePath has already been checked ResolvePath
	return os.ReadFile(safePath)
}

// SafeStat gets information about a file inside the sandbox.
func (s *SafeFSOps) SafeStat(userPath string) (os.FileInfo, error) {
	safePath, err := s.ResolvePath(userPath)
	if err != nil {
		return nil, err
	}

	return os.Stat(safePath)
}

// EnsureDir creates a directory inside the sandbox.
func (s *SafeFSOps) EnsureDir(dirPath string) error {
	safePath, err := s.ResolvePath(dirPath)
	if err != nil {
		return err
	}

	return os.MkdirAll(safePath, 0700)
}
