package display

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/manash/imggen/pkg/models"
)

func TestDisplayer_Display_WithData(t *testing.T) {
	var buf bytes.Buffer
	d := New(&buf)

	img := &models.GeneratedImage{
		Data: []byte("test image data"),
	}

	err := d.Display(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "\x1b_G") {
		t.Error("output should contain Kitty escape sequence")
	}
}

func TestDisplayer_Display_WithURL(t *testing.T) {
	imageData := []byte("downloaded image data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(imageData)
	}))
	defer server.Close()

	var buf bytes.Buffer
	d := New(&buf)

	img := &models.GeneratedImage{
		URL: server.URL,
	}

	err := d.Display(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "\x1b_G") {
		t.Error("output should contain Kitty escape sequence")
	}
}

func TestDisplayer_Display_NoDataOrURL(t *testing.T) {
	var buf bytes.Buffer
	d := New(&buf)

	img := &models.GeneratedImage{}

	err := d.Display(context.Background(), img)
	if err == nil {
		t.Error("expected error for image with no data or URL")
	}
}

func TestDisplayer_Display_DownloadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var buf bytes.Buffer
	d := New(&buf)

	img := &models.GeneratedImage{
		URL: server.URL,
	}

	err := d.Display(context.Background(), img)
	if err == nil {
		t.Error("expected error for failed download")
	}
}

func TestDisplayer_DisplayAll(t *testing.T) {
	var buf bytes.Buffer
	d := New(&buf)

	resp := &models.Response{
		Images: []models.GeneratedImage{
			{Data: []byte("image 1")},
			{Data: []byte("image 2")},
		},
	}

	err := d.DisplayAll(context.Background(), resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	escCount := strings.Count(output, "\x1b_G")
	if escCount != 2 {
		t.Errorf("expected 2 escape sequences, got %d", escCount)
	}
}

func TestDisplayer_DisplayAll_Empty(t *testing.T) {
	var buf bytes.Buffer
	d := New(&buf)

	resp := &models.Response{
		Images: []models.GeneratedImage{},
	}

	err := d.DisplayAll(context.Background(), resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.Len() != 0 {
		t.Error("expected no output for empty response")
	}
}

func TestDisplayer_Display_PrefersData(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Write([]byte("from server"))
	}))
	defer server.Close()

	var buf bytes.Buffer
	d := New(&buf)

	img := &models.GeneratedImage{
		Data: []byte("local data"),
		URL:  server.URL,
	}

	err := d.Display(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if serverCalled {
		t.Error("should use local data instead of downloading")
	}
}

func TestIsTerminalSupported(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "no env vars",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name:     "kitty terminal program",
			envVars:  map[string]string{"TERM_PROGRAM": "kitty"},
			expected: true,
		},
		{
			name:     "ghostty terminal program",
			envVars:  map[string]string{"TERM_PROGRAM": "ghostty"},
			expected: true,
		},
		{
			name:     "iterm terminal program",
			envVars:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			expected: true,
		},
		{
			name:     "wezterm terminal program",
			envVars:  map[string]string{"TERM_PROGRAM": "WezTerm"},
			expected: true,
		},
		{
			name:     "kitty window id",
			envVars:  map[string]string{"KITTY_WINDOW_ID": "123"},
			expected: true,
		},
		{
			name:     "iterm session id",
			envVars:  map[string]string{"ITERM_SESSION_ID": "abc"},
			expected: true,
		},
		{
			name:     "term contains kitty",
			envVars:  map[string]string{"TERM": "xterm-kitty"},
			expected: true,
		},
		{
			name:     "term contains ghostty",
			envVars:  map[string]string{"TERM": "ghostty"},
			expected: true,
		},
		{
			name:     "unsupported terminal",
			envVars:  map[string]string{"TERM_PROGRAM": "gnome-terminal"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("TERM_PROGRAM")
			os.Unsetenv("KITTY_WINDOW_ID")
			os.Unsetenv("ITERM_SESSION_ID")
			os.Unsetenv("TERM")

			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			result := IsTerminalSupported()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
