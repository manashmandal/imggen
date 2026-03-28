package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

type releaseInfo struct {
	TagName string      `json:"tag_name"`
	Assets  []assetInfo `json:"assets"`
}

type assetInfo struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// SelfUpdate updates the current binary to the latest release from GitHub.
// currentVersion should be a semver string like "v1.2.3" or "1.2.3". w receives status output.
func SelfUpdate(currentVersion string, w io.Writer) error {
	if currentVersion == "dev" {
		return errors.New("dev builds cannot self-update")
	}

	fmt.Fprintf(w, "Checking for updates...\n")

	release, err := fetchRelease()
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}

	latest := normalize(release.TagName)
	current := normalize(currentVersion)

	if !semver.IsValid(current) {
		return fmt.Errorf("invalid current version: %s", currentVersion)
	}
	if !semver.IsValid(latest) {
		return fmt.Errorf("invalid latest version: %s", release.TagName)
	}

	if semver.Compare(current, latest) >= 0 {
		fmt.Fprintf(w, "Already up to date (%s)\n", currentVersion)
		return nil
	}

	// Find the correct asset for this platform.
	versionWithoutV := strings.TrimPrefix(latest, "v")
	expectedAsset := assetName(versionWithoutV)

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == expectedAsset {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (expected %s)", runtime.GOOS, runtime.GOARCH, expectedAsset)
	}

	fmt.Fprintf(w, "Downloading %s...\n", expectedAsset)
	archivePath, err := downloadAsset(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download asset: %w", err)
	}
	defer os.Remove(archivePath)

	fmt.Fprintf(w, "Extracting binary...\n")
	binaryPath, err := extractBinary(archivePath)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}
	defer os.Remove(binaryPath)

	fmt.Fprintf(w, "Replacing binary...\n")
	if err := replaceBinary(binaryPath); err != nil {
		return err
	}

	fmt.Fprintf(w, "Successfully updated from %s to %s\n", currentVersion, release.TagName)
	return nil
}

// fetchRelease fetches the full latest release info including assets from GitHub.
// It uses the package-level releaseURL var (overridable in tests).
func fetchRelease() (*releaseInfo, error) {
	resp, err := http.Get(releaseURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &release, nil
}

// assetName returns the expected archive filename for the given version on the
// current platform. Version should not have a "v" prefix (goreleaser strips it).
func assetName(version string) string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("imggen_%s_%s_%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

func downloadAsset(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "imggen-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write download: %w", err)
	}

	return tmpFile.Name(), nil
}

func extractBinary(archivePath string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath)
	}
	return extractFromTarGz(archivePath)
}

func extractFromTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	binaryName := "imggen"
	if runtime.GOOS == "windows" {
		binaryName = "imggen.exe"
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar entry: %w", err)
		}

		// The binary might be at the root or inside a directory.
		name := filepath.Base(header.Name)
		if name != binaryName {
			continue
		}

		tmpDir, err := os.MkdirTemp("", "imggen-extract-*")
		if err != nil {
			return "", err
		}

		outPath := filepath.Join(tmpDir, binaryName)
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			return "", err
		}

		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return "", fmt.Errorf("failed to extract binary: %w", err)
		}
		outFile.Close()

		return outPath, nil
	}

	return "", fmt.Errorf("binary %q not found in archive", binaryName)
}

func extractFromZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	binaryName := "imggen"
	if runtime.GOOS == "windows" {
		binaryName = "imggen.exe"
	}

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name != binaryName {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return "", err
		}

		tmpDir, err := os.MkdirTemp("", "imggen-extract-*")
		if err != nil {
			rc.Close()
			return "", err
		}

		outPath := filepath.Join(tmpDir, binaryName)
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			rc.Close()
			return "", err
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return "", fmt.Errorf("failed to extract binary: %w", err)
		}
		outFile.Close()
		rc.Close()

		return outPath, nil
	}

	return "", fmt.Errorf("binary %q not found in zip archive", binaryName)
}

func replaceBinary(newBinaryPath string) error {
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find current executable: %w", err)
	}

	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	backupPath := currentPath + ".old"

	// Rename current binary to backup.
	if err := os.Rename(currentPath, backupPath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: try running with sudo (e.g., sudo imggen update)")
		}
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary into place.
	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		// Attempt to restore backup.
		_ = os.Rename(backupPath, currentPath)
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: try running with sudo (e.g., sudo imggen update)")
		}
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Clean up backup. Not critical if this fails.
	_ = os.Remove(backupPath)

	return nil
}
