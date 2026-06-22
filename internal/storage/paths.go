// internal/storage/paths.go
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SafeFSOps provides secure file system operations,
// preventing Path Traversal (CWE-22) via sandboxing.
type SafeFSOps struct {
	root string
}

// NewSafeFSOps creates a sandbox with the specified root path.
// All operations will be limited to this directory.
func NewSafeFSOps(root string) (*SafeFSOps, error) {
	cleanRoot := filepath.Clean(root)

	// Check existence and permissions
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

// ResolvePath cleans and checks that the path is inside root.
func (s *SafeFSOps) ResolvePath(userPath string) (string, error) {
	// Step 1: Path cleaning (removes ../, ./, extra slashes)
	cleanPath := filepath.Clean(userPath)

	// Step 2: Making it absolute relative to root
	absPath := filepath.Join(s.root, cleanPath)

	// Step 3: Verify we haven't gone outside root
	rel, err := filepath.Rel(s.root, absPath)
	if err != nil {
		return "", fmt.Errorf("path resolution failed: %w", err)
	}

	// Forbid escaping through "../../../"
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
	// #nosec G304 — safePath already validated by ResolvePath
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

	// Ensure parent directory exists
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
	// #nosec G304 — safePath already validated by ResolvePath
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

// GetBaseDir determines the application's base directory based on the presence of a .portable flag.
// If a .portable file exists next to the executable, it uses a 'data' subdirectory there.
// Otherwise, it uses the standard XDG path: ~/.local/share/lastchance/
// The directory is automatically created with 0700 permissions.
func GetBaseDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("unable to get executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	portableFlag := filepath.Join(execDir, ".portable")
	if _, err := os.Stat(portableFlag); err == nil {
		// Portable mode
		baseDir := filepath.Join(execDir, "data")
		if err := os.MkdirAll(baseDir, 0700); err != nil {
			return "", fmt.Errorf("failed to create portable data directory: %w", err)
		}
		return baseDir, nil
	}

	// Standard mode: XDG_DATA_HOME or ~/.local/share
	var dataHome string
	if runtime.GOOS == "windows" {
		// On Windows we fall back to AppData/Local
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		dataHome = localAppData
	} else {
		// Unix-like: respect XDG_DATA_HOME, default to ~/.local/share
		dataHome = os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("cannot find home directory: %w", err)
			}
			dataHome = filepath.Join(home, ".local", "share")
		}
	}

	baseDir := filepath.Join(dataHome, "lastchance")
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create app data directory: %w", err)
	}

	return baseDir, nil
}
