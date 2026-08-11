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

	// Normalize architecture names if needed (Go names are usually fine)
	// but let's be explicit for common release patterns.
	switch arch {
	case "amd64":
	case "arm64":
	default:
		return nil, fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Default destination
	destDir := "/usr/local/bin"
	if osName == "windows" {
		destDir = filepath.Join(os.Getenv("APPDATA"), "automergent", "bin")
	}

	return &Info{
		OS:      osName,
		Arch:    arch,
		DestDir: destDir,
	}, nil
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
