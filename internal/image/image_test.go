package image

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manash/imggen/internal/security"
	"github.com/manash/imggen/pkg/models"
)

func TestMain(m *testing.M) {
	// Disable URL validation for tests using httptest
	security.SetSkipValidation(true)
	code := m.Run()
	security.SetSkipValidation(false)
	os.Exit(code)
}

func TestNewSaver(t *testing.T) {
	s := NewSaver()
	if s == nil {
		t.Fatal("NewSaver() returned nil")
	}
	if s.httpClient == nil {
		t.Fatal("NewSaver() httpClient is nil")
	}
}

func TestSaver_Save_WithData(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")

	img := &models.GeneratedImage{
		Data:  []byte("fake image data"),
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(data) != "fake image data" {
		t.Errorf("saved data mismatch: got %s", string(data))
	}

	// Verify filename was set
	if img.Filename != path {
		t.Errorf("img.Filename = %v, want %v", img.Filename, path)
	}
}

func TestSaver_Save_WithURL(t *testing.T) {
	expectedData := []byte("downloaded image content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(expectedData)
	}))
	defer server.Close()

	s := NewSaver()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "downloaded.png")

	img := &models.GeneratedImage{
		URL:   server.URL,
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(data) != string(expectedData) {
		t.Errorf("saved data mismatch")
	}
}

func TestSaver_Save_NoData(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.png")

	img := &models.GeneratedImage{
		Index: 0,
		// No Data and no URL
	}

	err := s.Save(context.Background(), img, path)
	if err == nil {
		t.Fatal("Save() error = nil, want error for no data")
	}
}

func TestSaver_Save_DownloadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s := NewSaver()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error.png")

	img := &models.GeneratedImage{
		URL:   server.URL,
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err == nil {
		t.Fatal("Save() error = nil, want error for download failure")
	}
}

func TestSaver_Save_CreatesDirectory(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "test.png")

	img := &models.GeneratedImage{
		Data:  []byte("data"),
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Save() did not create nested directory")
	}
}

func TestSaver_Save_CurrentDirectory(t *testing.T) {
	s := NewSaver()
	// Use a temp file in current directory style
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	path := "test.png" // No directory component

	img := &models.GeneratedImage{
		Data:  []byte("data"),
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	os.Remove(path) // Cleanup
}

func TestSaver_SaveAll(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "image.png")

	resp := &models.Response{
		Images: []models.GeneratedImage{
			{Data: []byte("img1"), Index: 0},
			{Data: []byte("img2"), Index: 1},
			{Data: []byte("img3"), Index: 2},
		},
	}

	paths, err := s.SaveAll(context.Background(), resp, basePath, models.FormatPNG)
	if err != nil {
		t.Fatalf("SaveAll() error = %v", err)
	}

	if len(paths) != 3 {
		t.Errorf("SaveAll() returned %d paths, want 3", len(paths))
	}

	// Verify files exist
	for _, path := range paths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("SaveAll() file not created: %s", path)
		}
	}
}

func TestSaver_SaveAll_SingleImage(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "single.png")

	resp := &models.Response{
		Images: []models.GeneratedImage{
			{Data: []byte("img1"), Index: 0},
		},
	}

	paths, err := s.SaveAll(context.Background(), resp, basePath, models.FormatPNG)
	if err != nil {
		t.Fatalf("SaveAll() error = %v", err)
	}

	if len(paths) != 1 {
		t.Errorf("SaveAll() returned %d paths, want 1", len(paths))
	}

	if paths[0] != basePath {
		t.Errorf("SaveAll() path = %s, want %s", paths[0], basePath)
	}
}

func TestSaver_SaveAll_NoBasePath(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	resp := &models.Response{
		Images: []models.GeneratedImage{
			{Data: []byte("img1"), Index: 0},
			{Data: []byte("img2"), Index: 1},
		},
	}

	paths, err := s.SaveAll(context.Background(), resp, "", models.FormatPNG)
	if err != nil {
		t.Fatalf("SaveAll() error = %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("SaveAll() returned %d paths, want 2", len(paths))
	}

	// Verify auto-generated filenames
	for _, path := range paths {
		if !strings.HasPrefix(filepath.Base(path), "image-") {
			t.Errorf("SaveAll() unexpected filename: %s", path)
		}
	}
}

func TestSaver_SaveAll_Error(t *testing.T) {
	s := NewSaver()

	resp := &models.Response{
		Images: []models.GeneratedImage{
			{Index: 0}, // No data
		},
	}

	_, err := s.SaveAll(context.Background(), resp, "/tmp/test.png", models.FormatPNG)
	if err == nil {
		t.Fatal("SaveAll() error = nil, want error")
	}
}

func TestGenerateFilename(t *testing.T) {
	tests := []struct {
		index  int
		format models.OutputFormat
		check  func(string) bool
	}{
		{
			index:  0,
			format: models.FormatPNG,
			check: func(name string) bool {
				return strings.HasPrefix(name, "image-") && strings.HasSuffix(name, ".png") && !strings.Contains(name, "-1.")
			},
		},
		{
			index:  1,
			format: models.FormatJPEG,
			check: func(name string) bool {
				return strings.HasPrefix(name, "image-") && strings.HasSuffix(name, "-2.jpeg")
			},
		},
		{
			index:  2,
			format: models.FormatWebP,
			check: func(name string) bool {
				return strings.HasPrefix(name, "image-") && strings.HasSuffix(name, "-3.webp")
			},
		},
	}

	for _, tt := range tests {
		name := GenerateFilename(tt.index, tt.format)
		if !tt.check(name) {
			t.Errorf("GenerateFilename(%d, %s) = %s, unexpected format", tt.index, tt.format, name)
		}
	}
}

func TestGenerateFilenameWithTime(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)

	tests := []struct {
		index  int
		format models.OutputFormat
		want   string
	}{
		{0, models.FormatPNG, "image-20250115-103045.png"},
		{1, models.FormatJPEG, "image-20250115-103045-2.jpeg"},
		{2, models.FormatWebP, "image-20250115-103045-3.webp"},
	}

	for _, tt := range tests {
		got := GenerateFilenameWithTime(tt.index, tt.format, fixedTime)
		if got != tt.want {
			t.Errorf("GenerateFilenameWithTime(%d, %s, time) = %s, want %s", tt.index, tt.format, got, tt.want)
		}
	}
}

func TestSaver_generatePath(t *testing.T) {
	s := NewSaver()

	tests := []struct {
		name     string
		basePath string
		index    int
		total    int
		format   models.OutputFormat
		check    func(string) bool
	}{
		{
			name:     "single with base path",
			basePath: "/tmp/output.png",
			index:    0,
			total:    1,
			format:   models.FormatPNG,
			check:    func(p string) bool { return p == "/tmp/output.png" },
		},
		{
			name:     "multiple with base path first",
			basePath: "/tmp/output.png",
			index:    0,
			total:    3,
			format:   models.FormatPNG,
			check:    func(p string) bool { return p == "/tmp/output-1.png" },
		},
		{
			name:     "multiple with base path second",
			basePath: "/tmp/output.png",
			index:    1,
			total:    3,
			format:   models.FormatPNG,
			check:    func(p string) bool { return p == "/tmp/output-2.png" },
		},
		{
			name:     "no base path",
			basePath: "",
			index:    0,
			total:    1,
			format:   models.FormatPNG,
			check:    func(p string) bool { return strings.HasPrefix(p, "image-") && strings.HasSuffix(p, ".png") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.generatePath(tt.basePath, tt.index, tt.total, tt.format)
			if !tt.check(got) {
				t.Errorf("generatePath() = %s, unexpected", got)
			}
		})
	}
}

func TestSaver_downloadFromURL(t *testing.T) {
	expectedData := []byte("test content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(expectedData)
	}))
	defer server.Close()

	s := NewSaver()
	data, err := s.downloadFromURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("downloadFromURL() error = %v", err)
	}
	if string(data) != string(expectedData) {
		t.Errorf("downloadFromURL() data mismatch")
	}
}

func TestSaver_downloadFromURL_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	s := NewSaver()
	_, err := s.downloadFromURL(context.Background(), server.URL)
	if err == nil {
		t.Fatal("downloadFromURL() error = nil, want error")
	}
}

func TestSaver_downloadFromURL_InvalidURL(t *testing.T) {
	s := NewSaver()
	_, err := s.downloadFromURL(context.Background(), "not-a-valid-url")
	if err == nil {
		t.Fatal("downloadFromURL() error = nil, want error for invalid URL")
	}
}

func TestSaver_ensureDir(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()

	tests := []struct {
		name string
		path string
	}{
		{"nested path", filepath.Join(tmpDir, "a", "b", "c", "file.txt")},
		{"current dir", "file.txt"},
		{"dot dir", "./file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.ensureDir(tt.path)
			if err != nil {
				t.Errorf("ensureDir(%s) error = %v", tt.path, err)
			}
		})
	}
}

func TestSaver_Save_WriteError(t *testing.T) {
	s := NewSaver()
	// Try to write to a path that should fail (directory as file)
	tmpDir := t.TempDir()
	path := tmpDir // This is a directory, not a file

	img := &models.GeneratedImage{
		Data:  []byte("data"),
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err == nil {
		t.Fatal("Save() error = nil, want error for invalid path")
	}
}

func TestSaver_downloadFromURL_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("data"))
	}))
	defer server.Close()

	s := NewSaver()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.downloadFromURL(ctx, server.URL)
	if err == nil {
		t.Fatal("downloadFromURL() error = nil, want error for canceled context")
	}
}

func TestSaver_Save_WithDataPreferred(t *testing.T) {
	// When both Data and URL are present, Data should be used
	s := NewSaver()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")

	img := &models.GeneratedImage{
		Data:  []byte("direct data"),
		URL:   "http://should-not-be-called.invalid",
		Index: 0,
	}

	err := s.Save(context.Background(), img, path)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "direct data" {
		t.Error("Save() should prefer Data over URL")
	}
}

func TestSaver_SaveAll_PartialFailure(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()

	resp := &models.Response{
		Images: []models.GeneratedImage{
			{Data: []byte("img1"), Index: 0},
			{Index: 1}, // No data, will fail
		},
	}

	paths, err := s.SaveAll(context.Background(), resp, filepath.Join(tmpDir, "batch.png"), models.FormatPNG)
	if err == nil {
		t.Fatal("SaveAll() error = nil, want error for partial failure")
	}

	// First image should have been saved
	if len(paths) != 1 {
		t.Errorf("SaveAll() should have saved 1 image before failure, got %d", len(paths))
	}
}

func TestGenerateFilename_AllFormats(t *testing.T) {
	formats := []models.OutputFormat{models.FormatPNG, models.FormatJPEG, models.FormatWebP}
	for _, format := range formats {
		name := GenerateFilename(0, format)
		if !strings.HasSuffix(name, "."+string(format)) {
			t.Errorf("GenerateFilename(0, %s) = %s, wrong extension", format, name)
		}
	}
}
