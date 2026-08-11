package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const Repo = "iSundram/Automergent"

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a GitHub release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GetLatestVersion fetches the latest version string from GitHub.
func GetLatestVersion() (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch latest release: %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}

	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// DownloadBinary downloads the binary for the given version and system info.
func DownloadBinary(version string, info *Info, progressChan chan float64) (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}

	// Match asset name: automergent_0.1.0_linux_amd64.tar.gz
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}

	targetSuffix := fmt.Sprintf("%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	var downloadURL string
	for _, asset := range rel.Assets {
		if strings.HasSuffix(asset.Name, targetSuffix) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no matching asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Download the asset
	resp, err = http.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download: %s", resp.Status)
	}

	tmpFile, err := os.CreateTemp("", "automergent-download-*."+ext)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// Track progress if channel is provided
	var reader io.Reader = resp.Body
	if progressChan != nil {
		reader = &progressReader{
			Reader:   resp.Body,
			Total:    float64(resp.ContentLength),
			Progress: progressChan,
		}
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

type progressReader struct {
	io.Reader
	Total    float64
	Current  float64
	Progress chan float64
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	pr.Current += float64(n)
	if pr.Total > 0 && pr.Progress != nil {
		pr.Progress <- pr.Current / pr.Total
	}
	return
}

// ExtractBinary extracts the binary from the archive to the destination.
func ExtractBinary(archivePath, destDir string) error {
	ext := filepath.Ext(archivePath)
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, destDir)
	} else if ext == ".zip" {
		return extractZip(archivePath, destDir)
	}
	return fmt.Errorf("unsupported archive format: %s", ext)
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Only extract the 'automergent' binary
		if header.Typeflag == tar.TypeReg && (header.Name == "automergent" || header.Name == "automergent.exe") {
			target := filepath.Join(destDir, header.Name)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "automergent" || f.Name == "automergent.exe" {
			target := filepath.Join(destDir, f.Name)
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			dstFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			srcFile, err := f.Open()
			if err != nil {
				dstFile.Close()
				return err
			}
			if _, err := io.Copy(dstFile, srcFile); err != nil {
				srcFile.Close()
				dstFile.Close()
				return err
			}
			srcFile.Close()
			dstFile.Close()
		}
	}
	return nil
}
