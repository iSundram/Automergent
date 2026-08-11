package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Info holds system information relevant for installation.
type Info struct {
	OS      string
	Arch    string
	DestDir string
}

// GetSystemInfo detects the current OS and architecture.
func GetSystemInfo() (*Info, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	switch arch {
	case "amd64":
	case "arm64":
	default:
		return nil, fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Default destination with permission check
	destDir := "/usr/local/bin"
	if osName == "windows" {
		destDir = filepath.Join(os.Getenv("APPDATA"), "automergent", "bin")
	} else {
		// On Unix, check if /usr/local/bin is writable, otherwise fallback to ~/.local/bin
		if !isDirWritable(destDir) {
			home, _ := os.UserHomeDir()
			destDir = filepath.Join(home, ".local", "bin")
		}
	}

	return &Info{
		OS:      osName,
		Arch:    arch,
		DestDir: destDir,
	}, nil
}

func isDirWritable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		// If it doesn't exist, check if we can create it
		err := os.MkdirAll(path, 0755)
		return err == nil
	}
	if !info.IsDir() {
		return false
	}

	// Check write permission by creating a temporary file
	tmpFile, err := os.CreateTemp(path, ".write-test")
	if err != nil {
		return false
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPath)
	return true
}

// IsRoot checks if the process has administrative privileges.
func IsRoot() bool {
	if runtime.GOOS == "windows" {
		// Simplified check for Windows
		_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		return err == nil
	}
	return os.Geteuid() == 0
}

// AddToPath suggests or adds the destination directory to the PATH.
func AddToPath(destDir string) error {
	shell := os.Getenv("SHELL")
	home, _ := os.UserHomeDir()

	var rcFile string
	if strings.Contains(shell, "zsh") {
		rcFile = filepath.Join(home, ".zshrc")
	} else if strings.Contains(shell, "bash") {
		rcFile = filepath.Join(home, ".bashrc")
	}

	if rcFile == "" {
		return fmt.Errorf("could not detect shell config file")
	}

	content, err := os.ReadFile(rcFile)
	if err != nil {
		return err
	}

	pathEntry := fmt.Sprintf("\nexport PATH=\"$PATH:%s\"\n", destDir)
	if strings.Contains(string(content), destDir) {
		return nil // Already in path
	}

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(pathEntry)
	return err
}

// CheckBinary checks if a binary exists in the PATH.
func CheckBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// SetupBinary creates a symlink for the owe command alias.
func SetupBinary(destDir string) error {
	automergentExe := filepath.Join(destDir, "automergent")
	oweExe := filepath.Join(destDir, "owe")

	// On Windows, use .exe extension
	if runtime.GOOS == "windows" {
		automergentExe += ".exe"
		oweExe += ".exe"
	}

	// Check if automergent binary exists
	if _, err := os.Stat(automergentExe); err != nil {
		return fmt.Errorf("automergent binary not found at %s", automergentExe)
	}

	// Remove existing owe symlink if it exists
	os.Remove(oweExe)

	// Create symlink
	if err := os.Symlink(automergentExe, oweExe); err != nil {
		// On some systems symlink might fail, try copying instead
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	return nil
}
