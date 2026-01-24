package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestProvider_GenerateVideo_Success(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("wrong authorization header")
			}

			resp := videoJobResponse{
				ID:        "video_123",
				Object:    "video",
				CreatedAt: time.Now().Unix(),
				Status:    "queued",
				Model:     "sora-2",
				Progress:  0,
				Seconds:   4,
				Size:      "1280x720",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_123") && !strings.HasSuffix(r.URL.Path, "/content"):
			requestCount++
			status := "in_progress"
			progress := 50
			if requestCount >= 2 {
				status = "completed"
				progress = 100
			}

			resp := videoJobResponse{
				ID:        "video_123",
				Object:    "video",
				CreatedAt: time.Now().Unix(),
				Status:    status,
				Model:     "sora-2",
				Progress:  progress,
				Seconds:   4,
				Size:      "1280x720",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/content"):
			w.Header().Set("Content-Type", "video/mp4")
			w.Write([]byte("fake video data"))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	p, err := New(cfg, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Override poll interval for tests
	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{
		Model:    "sora-2",
		Prompt:   "a cat playing with yarn",
		Duration: 4,
		Size:     "1280x720",
	}

	resp, err := p.GenerateVideo(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateVideo() error = %v", err)
	}

	if len(resp.Videos) == 0 {
		t.Fatal("GenerateVideo() returned no videos")
	}
	if string(resp.Videos[0].Data) != "fake video data" {
		t.Errorf("GenerateVideo() video data mismatch")
	}
	if resp.Cost == nil {
		t.Error("GenerateVideo() cost is nil")
	}
}

func TestProvider_GenerateVideo_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := videoJobResponse{
			Error: &apiError{
				Message: "content policy violation",
				Type:    "invalid_request_error",
				Code:    "content_policy_violation",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.VideoRequest{
		Model:  "sora-2",
		Prompt: "test prompt",
	}

	_, err := p.GenerateVideo(context.Background(), req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "content policy violation") {
		t.Errorf("GenerateVideo() error = %v, want content policy error", err)
	}
}

func TestProvider_GenerateVideo_PollFailed(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			resp := videoJobResponse{
				ID:     "video_456",
				Status: "queued",
			}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_456"):
			requestCount++
			resp := videoJobResponse{
				ID:     "video_456",
				Status: "failed",
				Error: &apiError{
					Message: "generation failed",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{
		Model:  "sora-2",
		Prompt: "test",
	}

	_, err := p.GenerateVideo(context.Background(), req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error for failed status")
	}
	if !strings.Contains(err.Error(), "generation failed") {
		t.Errorf("GenerateVideo() error = %v, want generation failed error", err)
	}
}

func TestProvider_GenerateVideo_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := videoJobResponse{
			ID:     "video_789",
			Status: "queued",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req := &models.VideoRequest{
		Model:  "sora-2",
		Prompt: "test",
	}

	_, err := p.GenerateVideo(ctx, req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error for canceled context")
	}
}

func TestProvider_GenerateVideo_DownloadFailed(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			resp := videoJobResponse{ID: "video_dl", Status: "queued"}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_dl") && !strings.HasSuffix(r.URL.Path, "/content"):
			requestCount++
			resp := videoJobResponse{ID: "video_dl", Status: "completed"}
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(r.URL.Path, "/content"):
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{Model: "sora-2", Prompt: "test"}

	_, err := p.GenerateVideo(context.Background(), req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error for download failure")
	}
}

func TestProvider_SupportsVideoModel(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	tests := []struct {
		model string
		want  bool
	}{
		{"sora-2", true},
		{"sora-2-pro", true},
		{"gpt-image-1", false},
		{"unknown-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := p.SupportsVideoModel(tt.model); got != tt.want {
				t.Errorf("SupportsVideoModel(%s) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestProvider_ListVideoModels(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	videoModels := p.ListVideoModels()

	expected := map[string]bool{
		"sora-2":     true,
		"sora-2-pro": true,
	}

	if len(videoModels) != len(expected) {
		t.Errorf("ListVideoModels() returned %d models, want %d", len(videoModels), len(expected))
	}

	for _, model := range videoModels {
		if !expected[model] {
			t.Errorf("ListVideoModels() unexpected model: %s", model)
		}
	}
}

func TestProvider_GenerateVideo_Cost(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			resp := videoJobResponse{ID: "video_cost", Status: "queued", Seconds: 4}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_cost") && !strings.HasSuffix(r.URL.Path, "/content"):
			requestCount++
			resp := videoJobResponse{ID: "video_cost", Status: "completed", Seconds: 4}
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Write([]byte("video"))
		}
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{
		Model:    "sora-2",
		Prompt:   "test",
		Duration: 4,
	}

	resp, err := p.GenerateVideo(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateVideo() error = %v", err)
	}

	if resp.Cost == nil {
		t.Fatal("GenerateVideo() cost is nil")
	}

	// sora-2 is $0.10/second, 4 seconds = $0.40
	expectedTotal := 0.40
	if !floatEquals(resp.Cost.Total, expectedTotal) {
		t.Errorf("GenerateVideo() cost.Total = %f, want %f", resp.Cost.Total, expectedTotal)
	}
}

func TestProvider_GenerateVideo_UnknownStatus(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			resp := videoJobResponse{ID: "video_unknown", Status: "queued"}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_unknown"):
			requestCount++
			resp := videoJobResponse{ID: "video_unknown", Status: "unknown_status"}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{Model: "sora-2", Prompt: "test"}

	_, err := p.GenerateVideo(context.Background(), req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error for unknown status")
	}
	if !strings.Contains(err.Error(), "unknown video status") {
		t.Errorf("GenerateVideo() error = %v, want unknown status error", err)
	}
}

func TestProvider_GenerateVideo_HTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(videoJobResponse{})
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.VideoRequest{Model: "sora-2", Prompt: "test"}

	_, err := p.GenerateVideo(context.Background(), req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error for HTTP 500")
	}
}

func TestProvider_GenerateVideo_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.VideoRequest{Model: "sora-2", Prompt: "test"}

	_, err := p.GenerateVideo(context.Background(), req)
	if err == nil {
		t.Fatal("GenerateVideo() expected error for invalid JSON")
	}
}

func TestProvider_GenerateVideo_WithVerbose(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			resp := videoJobResponse{ID: "video_verbose", Status: "queued"}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_verbose") && !strings.HasSuffix(r.URL.Path, "/content"):
			requestCount++
			resp := videoJobResponse{ID: "video_verbose", Status: "completed"}
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Write([]byte("video"))
		}
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL, Verbose: true}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{Model: "sora-2", Prompt: "test"}

	_, err := p.GenerateVideo(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateVideo() with verbose error = %v", err)
	}
}

func TestProvider_GenerateVideo_WithSize(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			// Verify size is passed in form data
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			if r.FormValue("size") != "1920x1080" {
				t.Errorf("Expected size 1920x1080, got %s", r.FormValue("size"))
			}

			resp := videoJobResponse{ID: "video_size", Status: "queued"}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/videos/video_size") && !strings.HasSuffix(r.URL.Path, "/content"):
			requestCount++
			resp := videoJobResponse{ID: "video_size", Status: "completed"}
			json.NewEncoder(w).Encode(resp)

		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Write([]byte("video"))
		}
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	originalPollInterval := defaultPollInterval
	defaultPollInterval = 10 * time.Millisecond
	defer func() { defaultPollInterval = originalPollInterval }()

	req := &models.VideoRequest{
		Model:    "sora-2-pro",
		Prompt:   "test",
		Duration: 4,
		Size:     "1920x1080",
	}

	_, err := p.GenerateVideo(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateVideo() with size error = %v", err)
	}
}
