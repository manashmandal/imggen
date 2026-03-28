> **STATUS: DEPRECATED** — Sora video generation is shutting down September 24, 2026. See: https://help.openai.com/en/articles/20001152-what-to-know-about-the-sora-discontinuation

# OpenAI Sora Video Generation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add OpenAI Sora video generation support to the imggen CLI with models sora-2 and sora-2-pro.

**Architecture:** Extend the existing provider pattern with video-specific types and a new `video` subcommand. Video generation is async (job-based), requiring polling until completion before downloading the final MP4.

**Tech Stack:** Go, Cobra CLI, OpenAI Videos API (`/v1/videos` endpoints), HTTP client with polling

---

## API Reference (OpenAI Sora)

**Endpoints:**
- `POST /v1/videos` - Create video generation job (multipart/form-data)
- `GET /v1/videos/{video_id}` - Check job status
- `GET /v1/videos/{video_id}/content` - Download completed video

**Request Parameters (POST /v1/videos):**
| Parameter | Type | Required | Default | Values |
|-----------|------|----------|---------|--------|
| prompt | string | Yes | - | Text description |
| model | string | No | sora-2 | sora-2, sora-2-pro |
| seconds | int | No | 4 | 4, 8, 12 |
| size | string | No | 720x1280 | 720x1280, 1280x720, 1024x1792, 1792x1024 |

**Status Values:** `queued` → `in_progress` → `completed` | `failed`

**Models Available:**
- `sora-2` (default) - Fast, cost-effective
- `sora-2-pro` - Higher quality, longer render time

**Pricing (estimated):**
- sora-2: ~$0.10/second
- sora-2-pro: ~$0.30/second (720p), ~$0.50/second (1080p)

---

## Task 1: Add Video Models to Registry

**Files:**
- Modify: `/Users/manash/projects/imggen/pkg/models/models.go`
- Test: `/Users/manash/projects/imggen/pkg/models/models_test.go`

**Step 1: Write the failing test**

Create test file if not exists, add video model test:

```go
// In pkg/models/models_test.go (add to existing or create new)

func TestVideoModelCapabilities_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *VideoRequest
		cap     *VideoModelCapabilities
		wantErr error
	}{
		{
			name: "valid request",
			req: &VideoRequest{
				Prompt:   "a cat playing",
				Model:    "sora-2",
				Duration: 4,
				Size:     "1280x720",
			},
			cap: &VideoModelCapabilities{
				Name:              "sora-2",
				Provider:          ProviderOpenAI,
				SupportedDurations: []int{4, 8, 12},
				SupportedSizes:    []string{"720x1280", "1280x720", "1024x1792", "1792x1024"},
				DefaultDuration:   4,
				DefaultSize:       "720x1280",
			},
			wantErr: nil,
		},
		{
			name: "empty prompt",
			req: &VideoRequest{
				Prompt: "",
				Model:  "sora-2",
			},
			cap:     &VideoModelCapabilities{Name: "sora-2"},
			wantErr: ErrEmptyPrompt,
		},
		{
			name: "invalid duration",
			req: &VideoRequest{
				Prompt:   "test",
				Duration: 10,
			},
			cap: &VideoModelCapabilities{
				Name:              "sora-2",
				SupportedDurations: []int{4, 8, 12},
			},
			wantErr: ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate(tt.req)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultRegistry_VideoModels(t *testing.T) {
	r := DefaultRegistry()

	// Check sora-2 is registered
	cap, ok := r.GetVideo("sora-2")
	if !ok {
		t.Fatal("sora-2 not found in registry")
	}
	if cap.Provider != ProviderOpenAI {
		t.Errorf("sora-2 provider = %v, want %v", cap.Provider, ProviderOpenAI)
	}
	if cap.DefaultDuration != 4 {
		t.Errorf("sora-2 default duration = %d, want 4", cap.DefaultDuration)
	}

	// Check sora-2-pro is registered
	cap, ok = r.GetVideo("sora-2-pro")
	if !ok {
		t.Fatal("sora-2-pro not found in registry")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/manash/projects/imggen && go test ./pkg/models/... -v -run TestVideo`
Expected: FAIL - undefined types VideoRequest, VideoModelCapabilities, ErrInvalidDuration, GetVideo

**Step 3: Write minimal implementation**

Add to `pkg/models/models.go`:

```go
// Add new error
var (
	ErrInvalidDuration = errors.New("invalid duration for model")
)

// Video output format
type VideoFormat string

const (
	FormatMP4 VideoFormat = "mp4"
)

// VideoRequest represents a video generation request
type VideoRequest struct {
	Prompt   string
	Model    string
	Duration int    // seconds: 4, 8, or 12
	Size     string // e.g., "1280x720"
}

// NewVideoRequest creates a new video request with defaults
func NewVideoRequest(prompt string) *VideoRequest {
	return &VideoRequest{
		Prompt:   prompt,
		Duration: 4,
	}
}

// VideoResponse represents the result of video generation
type VideoResponse struct {
	Video         *GeneratedVideo
	RevisedPrompt string
	Cost          *CostInfo
}

// GeneratedVideo represents a generated video
type GeneratedVideo struct {
	ID       string
	Data     []byte
	URL      string
	Filename string
	Duration int
	Size     string
}

// VideoModelCapabilities defines what a video model supports
type VideoModelCapabilities struct {
	Name               string
	Provider           ProviderType
	SupportedDurations []int
	SupportedSizes     []string
	DefaultDuration    int
	DefaultSize        string
}

// Validate checks if a video request is valid for this model
func (c *VideoModelCapabilities) Validate(req *VideoRequest) error {
	if req.Prompt == "" {
		return ErrEmptyPrompt
	}

	if req.Duration != 0 && len(c.SupportedDurations) > 0 {
		valid := false
		for _, d := range c.SupportedDurations {
			if req.Duration == d {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("%w: %d not in %v", ErrInvalidDuration, req.Duration, c.SupportedDurations)
		}
	}

	if req.Size != "" && len(c.SupportedSizes) > 0 && !slices.Contains(c.SupportedSizes, req.Size) {
		return fmt.Errorf("%w: %q not in %v", ErrInvalidSize, req.Size, c.SupportedSizes)
	}

	return nil
}

// ApplyDefaults sets default values for unspecified fields
func (c *VideoModelCapabilities) ApplyDefaults(req *VideoRequest) {
	if req.Duration == 0 {
		req.Duration = c.DefaultDuration
	}
	if req.Size == "" {
		req.Size = c.DefaultSize
	}
	if req.Model == "" {
		req.Model = c.Name
	}
}
```

Add to `ModelRegistry` struct and methods:

```go
// Update ModelRegistry struct
type ModelRegistry struct {
	models      map[string]*ModelCapabilities
	ocrModels   map[string]*OCRModelCapabilities
	videoModels map[string]*VideoModelCapabilities  // Add this
}

// Update NewModelRegistry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:      make(map[string]*ModelCapabilities),
		ocrModels:   make(map[string]*OCRModelCapabilities),
		videoModels: make(map[string]*VideoModelCapabilities),  // Add this
	}
}

// Add video registry methods
func (r *ModelRegistry) RegisterVideo(cap *VideoModelCapabilities) {
	r.videoModels[cap.Name] = cap
}

func (r *ModelRegistry) GetVideo(name string) (*VideoModelCapabilities, bool) {
	cap, ok := r.videoModels[name]
	return cap, ok
}

func (r *ModelRegistry) ListVideoModels() []string {
	names := make([]string, 0, len(r.videoModels))
	for name := range r.videoModels {
		names = append(names, name)
	}
	return names
}
```

Add to `DefaultRegistry()`:

```go
// Video models (Sora series)
r.RegisterVideo(&VideoModelCapabilities{
	Name:               "sora-2",
	Provider:           ProviderOpenAI,
	SupportedDurations: []int{4, 8, 12},
	SupportedSizes:     []string{"720x1280", "1280x720", "1024x1792", "1792x1024"},
	DefaultDuration:    4,
	DefaultSize:        "720x1280",
})

r.RegisterVideo(&VideoModelCapabilities{
	Name:               "sora-2-pro",
	Provider:           ProviderOpenAI,
	SupportedDurations: []int{4, 8, 12},
	SupportedSizes:     []string{"720x1280", "1280x720", "1024x1792", "1792x1024"},
	DefaultDuration:    4,
	DefaultSize:        "720x1280",
})
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/manash/projects/imggen && go test ./pkg/models/... -v -run TestVideo`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/models/models.go pkg/models/models_test.go
git commit -m "$(cat <<'EOF'
feat(models): add video generation types for Sora models

Add VideoRequest, VideoResponse, GeneratedVideo, and VideoModelCapabilities
types to support OpenAI Sora video generation. Register sora-2 and sora-2-pro
models with their supported durations (4, 8, 12 seconds) and sizes.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add Video Pricing

**Files:**
- Modify: `/Users/manash/projects/imggen/internal/cost/pricing.go`
- Modify: `/Users/manash/projects/imggen/internal/cost/calculator.go`
- Test: `/Users/manash/projects/imggen/internal/cost/calculator_test.go`

**Step 1: Write the failing test**

Add to `calculator_test.go`:

```go
func TestCalculator_CalculateVideo(t *testing.T) {
	c := NewCalculator()

	tests := []struct {
		name     string
		model    string
		duration int
		wantCost float64
	}{
		{
			name:     "sora-2 4 seconds",
			model:    "sora-2",
			duration: 4,
			wantCost: 0.40, // $0.10/sec * 4
		},
		{
			name:     "sora-2 12 seconds",
			model:    "sora-2",
			duration: 12,
			wantCost: 1.20, // $0.10/sec * 12
		},
		{
			name:     "sora-2-pro 8 seconds",
			model:    "sora-2-pro",
			duration: 8,
			wantCost: 2.40, // $0.30/sec * 8
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := c.CalculateVideo(models.ProviderOpenAI, tt.model, tt.duration)
			if cost.Total != tt.wantCost {
				t.Errorf("CalculateVideo() total = %v, want %v", cost.Total, tt.wantCost)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/manash/projects/imggen && go test ./internal/cost/... -v -run TestCalculator_CalculateVideo`
Expected: FAIL - CalculateVideo undefined

**Step 3: Write minimal implementation**

Add to `pricing.go`:

```go
// Video pricing (USD per second)
var videoPricing = map[string]float64{
	"sora-2":     0.10, // $0.10 per second
	"sora-2-pro": 0.30, // $0.30 per second (720p estimate)
}

func GetVideoPricePerSecond(model string) (float64, bool) {
	price, ok := videoPricing[model]
	return price, ok
}
```

Add to `calculator.go`:

```go
// CalculateVideo calculates the cost for video generation
func (c *Calculator) CalculateVideo(provider models.ProviderType, model string, durationSeconds int) *models.CostInfo {
	var pricePerSecond float64

	switch provider {
	case models.ProviderOpenAI:
		price, ok := GetVideoPricePerSecond(model)
		if ok {
			pricePerSecond = price
		} else {
			pricePerSecond = 0.10 // default to sora-2 pricing
		}
	default:
		pricePerSecond = 0
	}

	total := pricePerSecond * float64(durationSeconds)

	return &models.CostInfo{
		PerImage: pricePerSecond, // Per-second cost stored here
		Total:    total,
		Currency: CurrencyUSD,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/manash/projects/imggen && go test ./internal/cost/... -v -run TestCalculator_CalculateVideo`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cost/pricing.go internal/cost/calculator.go internal/cost/calculator_test.go
git commit -m "$(cat <<'EOF'
feat(cost): add video generation pricing for Sora models

Add CalculateVideo method to cost calculator with pricing:
- sora-2: $0.10/second
- sora-2-pro: $0.30/second

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Implement Video Provider Interface

**Files:**
- Modify: `/Users/manash/projects/imggen/internal/provider/provider.go`

**Step 1: Write the failing test**

No separate test needed - this is interface definition only.

**Step 2: Add VideoProvider interface**

Add to `provider.go`:

```go
var (
	ErrVideoGenerationFailed = errors.New("video generation failed")
	ErrVideoNotReady         = errors.New("video not ready")
	ErrVideoDownloadFailed   = errors.New("video download failed")
)

// VideoProvider interface for video generation capabilities
type VideoProvider interface {
	GenerateVideo(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error)
	SupportsVideoModel(model string) bool
	ListVideoModels() []string
}
```

**Step 3: Commit**

```bash
git add internal/provider/provider.go
git commit -m "$(cat <<'EOF'
feat(provider): add VideoProvider interface

Define VideoProvider interface with GenerateVideo, SupportsVideoModel,
and ListVideoModels methods for video generation support.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Implement OpenAI Video Generation

**Files:**
- Create: `/Users/manash/projects/imggen/internal/provider/openai/video.go`
- Create: `/Users/manash/projects/imggen/internal/provider/openai/video_test.go`

**Step 1: Write the failing test**

Create `video_test.go`:

```go
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
	// Track request count for polling simulation
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos"):
			// Create video endpoint
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
			// Status check endpoint - simulate completion after 2 polls
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
			// Download endpoint
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

	if resp.Video == nil {
		t.Fatal("GenerateVideo() returned nil video")
	}
	if resp.Video.ID != "video_123" {
		t.Errorf("GenerateVideo() video ID = %s, want video_123", resp.Video.ID)
	}
	if string(resp.Video.Data) != "fake video data" {
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
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/manash/projects/imggen && go test ./internal/provider/openai/... -v -run TestProvider_GenerateVideo`
Expected: FAIL - GenerateVideo undefined

**Step 3: Write minimal implementation**

Create `video.go`:

```go
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

const (
	defaultPollInterval = 2 * time.Second
	maxPollAttempts     = 300 // 10 minutes max at 2s intervals
)

// Video API types
type videoJobResponse struct {
	ID        string    `json:"id"`
	Object    string    `json:"object"`
	CreatedAt int64     `json:"created_at"`
	Status    string    `json:"status"` // queued, in_progress, completed, failed
	Model     string    `json:"model"`
	Progress  int       `json:"progress"`
	Seconds   int       `json:"seconds,omitempty"`
	Size      string    `json:"size,omitempty"`
	Error     *apiError `json:"error,omitempty"`
}

// GenerateVideo generates a video using the Sora API
func (p *Provider) GenerateVideo(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	// Step 1: Create video generation job
	jobResp, err := p.createVideoJob(ctx, req)
	if err != nil {
		return nil, err
	}

	// Step 2: Poll until completion
	completedJob, err := p.pollVideoStatus(ctx, jobResp.ID)
	if err != nil {
		return nil, err
	}

	// Step 3: Download the video
	videoData, err := p.downloadVideo(ctx, completedJob.ID)
	if err != nil {
		return nil, err
	}

	// Build response
	response := &models.VideoResponse{
		Video: &models.GeneratedVideo{
			ID:       completedJob.ID,
			Data:     videoData,
			Duration: completedJob.Seconds,
			Size:     completedJob.Size,
		},
		Cost: p.costCalc.CalculateVideo(models.ProviderOpenAI, req.Model, req.Duration),
	}

	return response, nil
}

func (p *Provider) createVideoJob(ctx context.Context, req *models.VideoRequest) (*videoJobResponse, error) {
	// Build multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writer.WriteField("prompt", req.Prompt)
	writer.WriteField("model", req.Model)
	writer.WriteField("seconds", strconv.Itoa(req.Duration))
	if req.Size != "" {
		writer.WriteField("size", req.Size)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to create form: %w", err)
	}

	url := p.baseURL + "/videos"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logRequest(http.MethodPost, url, httpReq.Header, body.Bytes())

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	p.logResponse(resp.StatusCode, resp.Header, respBody)

	var jobResp videoJobResponse
	if err := json.Unmarshal(respBody, &jobResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if jobResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", provider.ErrVideoGenerationFailed, jobResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%w: status %d", provider.ErrVideoGenerationFailed, resp.StatusCode)
	}

	return &jobResp, nil
}

func (p *Provider) pollVideoStatus(ctx context.Context, videoID string) (*videoJobResponse, error) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for attempt := 0; attempt < maxPollAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			job, err := p.getVideoStatus(ctx, videoID)
			if err != nil {
				return nil, err
			}

			switch job.Status {
			case "completed":
				return job, nil
			case "failed":
				errMsg := "video generation failed"
				if job.Error != nil {
					errMsg = job.Error.Message
				}
				return nil, fmt.Errorf("%w: %s", provider.ErrVideoGenerationFailed, errMsg)
			case "queued", "in_progress":
				// Continue polling
				continue
			default:
				return nil, fmt.Errorf("unknown video status: %s", job.Status)
			}
		}
	}

	return nil, fmt.Errorf("%w: exceeded maximum poll attempts", provider.ErrVideoNotReady)
}

func (p *Provider) getVideoStatus(ctx context.Context, videoID string) (*videoJobResponse, error) {
	url := p.baseURL + "/videos/" + videoID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var jobResp videoJobResponse
	if err := json.Unmarshal(body, &jobResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &jobResp, nil
}

func (p *Provider) downloadVideo(ctx context.Context, videoID string) ([]byte, error) {
	url := p.baseURL + "/videos/" + videoID + "/content"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", provider.ErrVideoDownloadFailed, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// SupportsVideoModel checks if a video model is supported
func (p *Provider) SupportsVideoModel(model string) bool {
	cap, ok := p.registry.GetVideo(model)
	if !ok {
		return false
	}
	return cap.Provider == models.ProviderOpenAI
}

// ListVideoModels returns available video models
func (p *Provider) ListVideoModels() []string {
	var models []string
	for _, name := range p.registry.ListVideoModels() {
		cap, ok := p.registry.GetVideo(name)
		if ok && cap.Provider == models.ProviderOpenAI {
			models = append(models, name)
		}
	}
	return models
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/manash/projects/imggen && go test ./internal/provider/openai/... -v -run TestProvider_GenerateVideo`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/provider/openai/video.go internal/provider/openai/video_test.go
git commit -m "$(cat <<'EOF'
feat(openai): implement video generation with Sora API

Add GenerateVideo method that:
- Creates video job via POST /v1/videos (multipart/form-data)
- Polls GET /v1/videos/{id} until completion
- Downloads video via GET /v1/videos/{id}/content
- Calculates cost based on duration

Supports sora-2 and sora-2-pro models with configurable duration and size.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add Video Saver

**Files:**
- Modify: `/Users/manash/projects/imggen/internal/image/image.go`

**Step 1: Write the failing test**

Add to existing image tests or create new test:

```go
func TestSaver_SaveVideo(t *testing.T) {
	s := NewSaver()
	tmpDir := t.TempDir()

	video := &models.GeneratedVideo{
		ID:       "video_123",
		Data:     []byte("fake video content"),
		Duration: 4,
		Size:     "1280x720",
	}

	path := filepath.Join(tmpDir, "test.mp4")
	err := s.SaveVideo(context.Background(), video, path)
	if err != nil {
		t.Fatalf("SaveVideo() error = %v", err)
	}

	// Verify file exists
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if string(data) != "fake video content" {
		t.Errorf("saved content mismatch")
	}
	if video.Filename != path {
		t.Errorf("Filename = %s, want %s", video.Filename, path)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/manash/projects/imggen && go test ./internal/image/... -v -run TestSaver_SaveVideo`
Expected: FAIL - SaveVideo undefined

**Step 3: Write minimal implementation**

Add to `image.go`:

```go
// SaveVideo saves a generated video to disk
func (s *Saver) SaveVideo(ctx context.Context, video *models.GeneratedVideo, path string) error {
	var data []byte
	var err error

	if len(video.Data) > 0 {
		data = video.Data
	} else if video.URL != "" {
		data, err = s.downloadFromURL(ctx, video.URL)
		if err != nil {
			return fmt.Errorf("failed to download video: %w", err)
		}
	} else {
		return fmt.Errorf("no video data available")
	}

	if err := s.ensureDir(path); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	video.Filename = path
	return nil
}

// GenerateVideoFilename generates a filename for video output
func GenerateVideoFilename(index int) string {
	timestamp := time.Now().Format("20060102-150405")
	if index > 0 {
		return fmt.Sprintf("video-%s-%d.mp4", timestamp, index+1)
	}
	return fmt.Sprintf("video-%s.mp4", timestamp)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/manash/projects/imggen && go test ./internal/image/... -v -run TestSaver_SaveVideo`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/image/image.go internal/image/image_test.go
git commit -m "$(cat <<'EOF'
feat(image): add video saving support

Add SaveVideo method to Saver for saving generated videos to disk.
Add GenerateVideoFilename for consistent video file naming.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Add Video CLI Command

**Files:**
- Modify: `/Users/manash/projects/imggen/cmd/imggen/main.go`

**Step 1: Write the failing test**

Add to `main_test.go`:

```go
func TestRunVideo_Success(t *testing.T) {
	var out bytes.Buffer
	mockProv := &mockVideoProvider{
		generateVideoResp: &models.VideoResponse{
			Video: &models.GeneratedVideo{
				ID:       "video_123",
				Data:     []byte("fake video"),
				Duration: 4,
				Size:     "1280x720",
			},
			Cost: &models.CostInfo{
				Total:    0.40,
				Currency: "USD",
			},
		},
	}

	app := newTestApp(&out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return mockProv, nil
	}

	tmpDir := t.TempDir()
	resetFlags()
	flagModel = "sora-2"
	flagOutput = filepath.Join(tmpDir, "test.mp4")

	cmd := newVideoCmd(app)
	cmd.SetArgs([]string{"a cat playing"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Generating video") {
		t.Errorf("output missing 'Generating video': %s", output)
	}
	if !strings.Contains(output, "Saved:") {
		t.Errorf("output missing 'Saved:': %s", output)
	}
}

// Mock provider that implements VideoProvider
type mockVideoProvider struct {
	mockProvider
	generateVideoResp *models.VideoResponse
	generateVideoErr  error
}

func (m *mockVideoProvider) GenerateVideo(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	if m.generateVideoErr != nil {
		return nil, m.generateVideoErr
	}
	return m.generateVideoResp, nil
}

func (m *mockVideoProvider) SupportsVideoModel(model string) bool {
	return model == "sora-2" || model == "sora-2-pro"
}

func (m *mockVideoProvider) ListVideoModels() []string {
	return []string{"sora-2", "sora-2-pro"}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/manash/projects/imggen && go test ./cmd/imggen/... -v -run TestRunVideo`
Expected: FAIL - newVideoCmd undefined

**Step 3: Write minimal implementation**

Add video flags at the top with other flags:

```go
var (
	flagVideoDuration int
	flagVideoSize     string
	flagVideoModel    string
)
```

Add newVideoCmd function and runVideo:

```go
func newVideoCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "video [prompt]",
		Short: "Generate videos using AI video generation APIs",
		Long: `Generate videos using OpenAI's Sora API.

Supported models:
  - sora-2 (default): Fast, cost-effective video generation
  - sora-2-pro: Higher quality, longer render time

Examples:
  imggen video "a cat playing with yarn"
  imggen video -m sora-2-pro -d 8 "cinematic sunset over ocean"
  imggen video -s 1280x720 -o output.mp4 "city timelapse"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVideo(cmd, args, app)
		},
	}

	cmd.Flags().StringVarP(&flagVideoModel, "model", "m", "sora-2", "model to use (sora-2, sora-2-pro)")
	cmd.Flags().IntVarP(&flagVideoDuration, "duration", "d", 4, "video duration in seconds (4, 8, or 12)")
	cmd.Flags().StringVarP(&flagVideoSize, "size", "s", "", "video size (720x1280, 1280x720, 1024x1792, 1792x1024)")
	cmd.Flags().StringVarP(&flagOutput, "output", "o", "", "output filename (default: video-TIMESTAMP.mp4)")
	cmd.Flags().StringVar(&flagAPIKey, "api-key", "", "API key (defaults to OPENAI_API_KEY)")
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log HTTP requests and responses")

	return cmd
}

func runVideo(_ *cobra.Command, args []string, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Get API key
	apiKey, _, err := keys.GetAPIKey(flagAPIKey, "openai", "OPENAI_API_KEY")
	if err != nil {
		return err
	}

	prompt := args[0]

	req := models.NewVideoRequest(prompt)
	req.Model = flagVideoModel
	req.Duration = flagVideoDuration
	req.Size = flagVideoSize

	// Validate against model capabilities
	caps, ok := app.Registry.GetVideo(flagVideoModel)
	if !ok {
		return fmt.Errorf("unknown video model %q: available models: %v", flagVideoModel, app.Registry.ListVideoModels())
	}

	caps.ApplyDefaults(req)

	if err := caps.Validate(req); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	// Create provider
	providerCfg := &provider.Config{APIKey: apiKey, Verbose: flagVerbose}
	prov, err := app.NewProvider(providerCfg, app.Registry)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Check if provider supports video
	videoProv, ok := prov.(interface {
		GenerateVideo(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error)
	})
	if !ok {
		return fmt.Errorf("provider does not support video generation")
	}

	fmt.Fprintf(app.Out, "Generating video with %s (%ds, %s)...\n", req.Model, req.Duration, req.Size)

	resp, err := videoProv.GenerateVideo(ctx, req)
	if err != nil {
		return fmt.Errorf("video generation failed: %w", err)
	}

	// Determine output path
	outputPath := flagOutput
	if outputPath == "" {
		outputPath = image.GenerateVideoFilename(0)
	}

	// Save video
	saver := app.NewSaver()
	if err := saver.SaveVideo(ctx, resp.Video, outputPath); err != nil {
		return fmt.Errorf("failed to save video: %w", err)
	}

	fmt.Fprintf(app.Out, "Saved: %s\n", outputPath)

	// Show cost
	if resp.Cost != nil {
		fmt.Fprintf(app.Out, "Cost: $%.4f (%ds @ $%.4f/s, %s)\n",
			resp.Cost.Total, req.Duration, resp.Cost.PerImage, req.Model)

		// Log cost to database
		store, err := session.NewStore()
		if err == nil {
			defer store.Close()
			costEntry := &session.CostEntry{
				IterationID: "",
				SessionID:   "",
				Provider:    "openai",
				Model:       req.Model,
				Cost:        resp.Cost.Total,
				ImageCount:  1, // 1 video
				Timestamp:   time.Now(),
			}
			if logErr := store.LogCost(ctx, costEntry); logErr != nil {
				fmt.Fprintf(app.Err, "Warning: failed to log cost: %v\n", logErr)
			}
		}
	}

	fmt.Fprintln(app.Out, "Done!")
	return nil
}
```

Register the command in `newRootCmd`:

```go
// Add after other AddCommand calls
cmd.AddCommand(newVideoCmd(app))
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/manash/projects/imggen && go test ./cmd/imggen/... -v -run TestRunVideo`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/imggen/main.go cmd/imggen/main_test.go
git commit -m "$(cat <<'EOF'
feat(cli): add video subcommand for Sora video generation

Add 'imggen video' command with flags:
- -m/--model: sora-2 (default), sora-2-pro
- -d/--duration: 4, 8, or 12 seconds
- -s/--size: output resolution
- -o/--output: output filename

Usage: imggen video "a cat playing with yarn"

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Update Help Text and Documentation

**Files:**
- Modify: `/Users/manash/projects/imggen/cmd/imggen/main.go` (root command help)

**Step 1: Update root command help text**

Update the Long description in `newRootCmd`:

```go
Long: `imggen is a CLI tool for generating images and videos using AI APIs.

Supported providers:
  - OpenAI (gpt-image-1, dall-e-3, dall-e-2)

Video generation:
  - OpenAI Sora (sora-2, sora-2-pro)

Note: Only OpenAI is currently supported. Other providers (Stability AI, etc.) are work in progress.

Examples:
  imggen "a sunset over mountains"
  imggen -m dall-e-3 -s 1792x1024 -q hd "panoramic cityscape"
  imggen -m gpt-image-1 -n 3 --transparent "logo design"
  imggen --prompt "a sunset" --prompt "a cat" -o ./output
  imggen video "a cat playing with yarn"
  imggen video -m sora-2-pro -d 8 "cinematic sunset"
  imggen -i  # start interactive mode`,
```

**Step 2: Commit**

```bash
git add cmd/imggen/main.go
git commit -m "$(cat <<'EOF'
docs(cli): update help text with video generation examples

Add Sora video generation examples to root command help.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Run Full Test Suite

**Step 1: Run all tests**

Run: `cd /Users/manash/projects/imggen && go test ./... -v`
Expected: All tests PASS

**Step 2: Build and verify CLI**

Run: `cd /Users/manash/projects/imggen && go build -o imggen ./cmd/imggen && ./imggen --help`
Expected: Shows help with video command listed

Run: `./imggen video --help`
Expected: Shows video-specific help

**Step 3: Final commit with version bump (if tests pass)**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat: complete OpenAI Sora video generation support

Summary:
- Add sora-2 and sora-2-pro video models to registry
- Implement video generation with async polling
- Add 'imggen video' CLI command
- Add video cost tracking ($0.10/s for sora-2, $0.30/s for sora-2-pro)

Usage:
  imggen video "prompt" [-m model] [-d duration] [-s size] [-o output]

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Summary of Files Changed

| File | Action | Description |
|------|--------|-------------|
| `pkg/models/models.go` | Modify | Add VideoRequest, VideoResponse, VideoModelCapabilities, registry methods |
| `pkg/models/models_test.go` | Modify | Add video model tests |
| `internal/cost/pricing.go` | Modify | Add Sora video pricing |
| `internal/cost/calculator.go` | Modify | Add CalculateVideo method |
| `internal/cost/calculator_test.go` | Modify | Add video cost tests |
| `internal/provider/provider.go` | Modify | Add VideoProvider interface and errors |
| `internal/provider/openai/video.go` | Create | Implement Sora video generation |
| `internal/provider/openai/video_test.go` | Create | Video generation tests |
| `internal/image/image.go` | Modify | Add SaveVideo, GenerateVideoFilename |
| `cmd/imggen/main.go` | Modify | Add video command and flags |
| `cmd/imggen/main_test.go` | Modify | Add video command tests |

---

## CLI Usage After Implementation

```bash
# Generate 4-second video with default settings
imggen video "a cat playing with yarn"

# Generate 8-second HD video
imggen video -m sora-2-pro -d 8 "cinematic sunset over ocean"

# Specify output file and size
imggen video -s 1280x720 -d 12 -o myvideo.mp4 "city timelapse"

# With verbose logging
imggen video -v "astronaut on mars"
```

---

**Sources used for API reference:**
- [Video generation with Sora | OpenAI API](https://platform.openai.com/docs/guides/video-generation)
- [Sora 2 Model | OpenAI API](https://platform.openai.com/docs/models/sora-2)
- [OpenAI Video Generation | liteLLM](https://docs.litellm.ai/docs/providers/openai/videos)
