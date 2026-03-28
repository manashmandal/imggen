package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssetName_Format(t *testing.T) {
	name := assetName("1.2.3")

	// Should contain version, OS, and arch.
	if !strings.Contains(name, "1.2.3") {
		t.Errorf("expected version in asset name, got %s", name)
	}
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("expected GOOS %s in asset name, got %s", runtime.GOOS, name)
	}
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("expected GOARCH %s in asset name, got %s", runtime.GOARCH, name)
	}

	// Should follow the pattern imggen_{version}_{os}_{arch}.{ext}
	expected := "imggen_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		expected += ".zip"
	} else {
		expected += ".tar.gz"
	}
	if name != expected {
		t.Errorf("expected %s, got %s", expected, name)
	}
}

func TestAssetName_DarwinARM64(t *testing.T) {
	// We can't mock runtime.GOOS/GOARCH, but we can verify the function
	// produces the right format for the current platform. On darwin/arm64
	// this is a direct test; on other platforms it validates the format.
	name := assetName("0.9.0")

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		expected := "imggen_0.9.0_darwin_arm64.tar.gz"
		if name != expected {
			t.Errorf("expected %s, got %s", expected, name)
		}
	}

	// General format check regardless of platform.
	if !strings.HasPrefix(name, "imggen_0.9.0_") {
		t.Errorf("expected prefix imggen_0.9.0_, got %s", name)
	}
}

func TestSelfUpdate_DevVersion(t *testing.T) {
	err := SelfUpdate("dev", io.Discard)
	if err == nil {
		t.Fatal("expected error for dev version, got nil")
	}
	if err.Error() != "dev builds cannot self-update" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestSelfUpdate_AlreadyLatest(t *testing.T) {
	asset := assetName("1.0.0")
	release := releaseInfo{
		TagName: "v1.0.0",
		Assets: []assetInfo{
			{Name: asset, BrowserDownloadURL: "https://example.com/fake"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	// Override the package-level releaseURL to point at our test server.
	origURL := releaseURL
	releaseURL = server.URL
	defer func() { releaseURL = origURL }()

	var buf bytes.Buffer
	err := SelfUpdate("v1.0.0", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Already up to date") {
		t.Errorf("expected 'Already up to date' in output, got: %s", output)
	}
}

func TestSelfUpdate_InvalidVersion(t *testing.T) {
	release := releaseInfo{
		TagName: "v1.0.0",
		Assets:  []assetInfo{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	origURL := releaseURL
	releaseURL = server.URL
	defer func() { releaseURL = origURL }()

	err := SelfUpdate("not-semver", io.Discard)
	if err == nil {
		t.Fatal("expected error for invalid version, got nil")
	}
	if !strings.Contains(err.Error(), "invalid current version") {
		t.Errorf("expected 'invalid current version' in error, got: %s", err.Error())
	}
}

func TestExtractBinary_TarGz(t *testing.T) {
	// Create a real tar.gz archive containing a fake binary.
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	binaryName := "imggen"
	if runtime.GOOS == "windows" {
		binaryName = "imggen.exe"
	}
	binaryContent := []byte("#!/bin/sh\necho hello\n")

	// Build the tar.gz.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a README to make it more realistic (goreleaser includes extra files).
	readmeContent := []byte("# imggen\nA CLI tool.\n")
	if err := tw.WriteHeader(&tar.Header{
		Name: "imggen_1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + "/README.md",
		Mode: 0644,
		Size: int64(len(readmeContent)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(readmeContent); err != nil {
		t.Fatal(err)
	}

	// Add the binary inside a subdirectory (goreleaser convention).
	if err := tw.WriteHeader(&tar.Header{
		Name: "imggen_1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + "/" + binaryName,
		Mode: 0755,
		Size: int64(len(binaryContent)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	// Extract.
	extractedPath, err := extractFromTarGz(archivePath)
	if err != nil {
		t.Fatalf("extractFromTarGz failed: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(extractedPath))

	// Verify the extracted binary.
	if filepath.Base(extractedPath) != binaryName {
		t.Errorf("expected extracted file named %s, got %s", binaryName, filepath.Base(extractedPath))
	}

	content, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, binaryContent) {
		t.Errorf("extracted content mismatch: got %q, want %q", content, binaryContent)
	}

	// Verify it's executable.
	info, err := os.Stat(extractedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("extracted binary is not executable")
	}
}

func TestExtractBinary_TarGz_NoBinary(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "empty.tar.gz")

	// Build a tar.gz with no binary.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := []byte("just a readme")
	if err := tw.WriteHeader(&tar.Header{
		Name: "README.md",
		Mode: 0644,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	tw.Write(content)
	tw.Close()
	gw.Close()

	os.WriteFile(archivePath, buf.Bytes(), 0644)

	_, err := extractFromTarGz(archivePath)
	if err == nil {
		t.Fatal("expected error when binary not found in archive")
	}
	if !strings.Contains(err.Error(), "not found in archive") {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestFetchRelease_ParsesAssets(t *testing.T) {
	release := releaseInfo{
		TagName: "v2.0.0",
		Assets: []assetInfo{
			{Name: "imggen_2.0.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/dl"},
			{Name: "imggen_2.0.0_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/dl2"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	origURL := releaseURL
	releaseURL = server.URL
	defer func() { releaseURL = origURL }()

	got, err := fetchRelease()
	if err != nil {
		t.Fatalf("fetchRelease failed: %v", err)
	}
	if got.TagName != "v2.0.0" {
		t.Errorf("expected tag v2.0.0, got %s", got.TagName)
	}
	if len(got.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(got.Assets))
	}
	if got.Assets[1].BrowserDownloadURL != "https://example.com/dl2" {
		t.Errorf("unexpected URL: %s", got.Assets[1].BrowserDownloadURL)
	}
}

func TestFetchRelease_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origURL := releaseURL
	releaseURL = server.URL
	defer func() { releaseURL = origURL }()

	_, err := fetchRelease()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected status 500 in error, got: %s", err.Error())
	}
}

func TestReplaceBinary_RenameLogic(t *testing.T) {
	// Test the rename logic with temp files. We can't call replaceBinary
	// directly since it uses os.Executable(), so we test the underlying
	// rename sequence that replaceBinary performs.
	tmpDir := t.TempDir()

	currentPath := filepath.Join(tmpDir, "imggen")
	if err := os.WriteFile(currentPath, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	newPath := filepath.Join(tmpDir, "imggen-new")
	if err := os.WriteFile(newPath, []byte("new-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	backupPath := currentPath + ".old"

	// Simulate replaceBinary logic.
	if err := os.Rename(currentPath, backupPath); err != nil {
		t.Fatalf("backup rename failed: %v", err)
	}
	if err := os.Rename(newPath, currentPath); err != nil {
		_ = os.Rename(backupPath, currentPath) // restore
		t.Fatalf("install rename failed: %v", err)
	}
	_ = os.Remove(backupPath)

	// Verify new binary is in place.
	content, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new-binary" {
		t.Errorf("expected 'new-binary', got %q", string(content))
	}

	// Verify backup was cleaned up.
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("expected backup file to be removed")
	}
}
