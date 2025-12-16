package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestNew(t *testing.T) {
	registry := models.DefaultRegistry()

	tests := []struct {
		name    string
		cfg     *provider.Config
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     &provider.Config{APIKey: "test-key"},
			wantErr: nil,
		},
		{
			name:    "empty API key",
			cfg:     &provider.Config{APIKey: ""},
			wantErr: provider.ErrAPIKeyRequired,
		},
		{
			name:    "custom base URL",
			cfg:     &provider.Config{APIKey: "test-key", BaseURL: "https://custom.api.com"},
			wantErr: nil,
		},
		{
			name:    "custom timeout",
			cfg:     &provider.Config{APIKey: "test-key", TimeoutSec: 60},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(tt.cfg, registry)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("New() error = nil, want error")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("New() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
			if p == nil {
				t.Fatal("New() returned nil provider")
			}
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	if p.Name() != models.ProviderOpenAI {
		t.Errorf("Name() = %v, want %v", p.Name(), models.ProviderOpenAI)
	}
}

func TestProvider_SupportsModel(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	tests := []struct {
		model string
		want  bool
	}{
		{"gpt-image-1", true},
		{"dall-e-3", true},
		{"dall-e-2", true},
		{"stable-diffusion-xl", false},
		{"unknown-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := p.SupportsModel(tt.model); got != tt.want {
				t.Errorf("SupportsModel(%s) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestProvider_ListModels(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	modelsList := p.ListModels()

	expected := map[string]bool{
		"gpt-image-1": true,
		"dall-e-3":    true,
		"dall-e-2":    true,
	}

	if len(modelsList) != len(expected) {
		t.Errorf("ListModels() returned %d models, want %d", len(modelsList), len(expected))
	}

	for _, model := range modelsList {
		if !expected[model] {
			t.Errorf("ListModels() unexpected model: %s", model)
		}
	}
}

func TestProvider_Generate_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("wrong content type")
		}

		// Parse request body
		var req apiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Prompt != "test prompt" {
			t.Errorf("wrong prompt: %s", req.Prompt)
		}

		// Send response
		resp := apiResponse{
			Created: time.Now().Unix(),
			Data: []imageData{
				{
					B64JSON:       base64.StdEncoding.EncodeToString([]byte("fake image data")),
					RevisedPrompt: "revised test prompt",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
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

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test prompt",
		Count:  1,
		Size:   "1024x1024",
		Format: models.FormatPNG,
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(resp.Images) != 1 {
		t.Errorf("Generate() returned %d images, want 1", len(resp.Images))
	}
	if resp.RevisedPrompt != "revised test prompt" {
		t.Errorf("Generate() RevisedPrompt = %s, want 'revised test prompt'", resp.RevisedPrompt)
	}
	if string(resp.Images[0].Data) != "fake image data" {
		t.Errorf("Generate() image data mismatch")
	}
}

func TestProvider_Generate_WithURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Created: time.Now().Unix(),
			Data: []imageData{
				{
					URL: "https://example.com/image.png",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "dall-e-3",
		Prompt: "test",
		Count:  1,
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if resp.Images[0].URL != "https://example.com/image.png" {
		t.Errorf("Generate() URL = %s, want https://example.com/image.png", resp.Images[0].URL)
	}
}

func TestProvider_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Error: &apiError{
				Message: "invalid prompt",
				Type:    "invalid_request_error",
				Code:    "invalid_prompt",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  1,
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
	if !errors.Is(err, provider.ErrGenerationFailed) {
		t.Errorf("Generate() error = %v, want %v", err, provider.ErrGenerationFailed)
	}
}

func TestProvider_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(apiResponse{})
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  1,
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func TestProvider_Generate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  1,
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func TestProvider_Generate_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(apiResponse{})
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  1,
	}

	_, err := p.Generate(ctx, req)
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
}

func TestProvider_buildAPIRequest_GPTImage1(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:       "gpt-image-1",
		Prompt:      "test prompt",
		Count:       2,
		Size:        "1024x1024",
		Quality:     "high",
		Format:      models.FormatPNG,
		Transparent: true,
	}

	apiReq := p.buildAPIRequest(req)

	if apiReq.Model != "gpt-image-1" {
		t.Errorf("buildAPIRequest() Model = %v", apiReq.Model)
	}
	if apiReq.OutputFormat != "png" {
		t.Errorf("buildAPIRequest() OutputFormat = %v, want png", apiReq.OutputFormat)
	}
	if apiReq.Background != "transparent" {
		t.Errorf("buildAPIRequest() Background = %v, want transparent", apiReq.Background)
	}
	if apiReq.ResponseFormat != "" {
		t.Errorf("buildAPIRequest() ResponseFormat should be empty for gpt-image-1")
	}
}

func TestProvider_buildAPIRequest_DallE3(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:   "dall-e-3",
		Prompt:  "test prompt",
		Count:   1,
		Size:    "1024x1024",
		Quality: "hd",
		Style:   "vivid",
	}

	apiReq := p.buildAPIRequest(req)

	if apiReq.ResponseFormat != "url" {
		t.Errorf("buildAPIRequest() ResponseFormat = %v, want url", apiReq.ResponseFormat)
	}
	if apiReq.Style != "vivid" {
		t.Errorf("buildAPIRequest() Style = %v, want vivid", apiReq.Style)
	}
}

func TestProvider_buildAPIRequest_DallE2(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:  "dall-e-2",
		Prompt: "test prompt",
		Count:  5,
		Size:   "512x512",
	}

	apiReq := p.buildAPIRequest(req)

	if apiReq.ResponseFormat != "url" {
		t.Errorf("buildAPIRequest() ResponseFormat = %v, want url", apiReq.ResponseFormat)
	}
	if apiReq.N != 5 {
		t.Errorf("buildAPIRequest() N = %v, want 5", apiReq.N)
	}
}

func TestProvider_buildResponse(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	encodedImage := base64.StdEncoding.EncodeToString([]byte("test image"))
	apiResp := apiResponse{
		Created: time.Now().Unix(),
		Data: []imageData{
			{B64JSON: encodedImage, RevisedPrompt: "revised"},
			{URL: "https://example.com/img.png"},
		},
	}

	resp, err := p.buildResponse(apiResp)
	if err != nil {
		t.Fatalf("buildResponse() error = %v", err)
	}

	if len(resp.Images) != 2 {
		t.Errorf("buildResponse() returned %d images, want 2", len(resp.Images))
	}
	if resp.RevisedPrompt != "revised" {
		t.Errorf("buildResponse() RevisedPrompt = %s, want revised", resp.RevisedPrompt)
	}
	if string(resp.Images[0].Data) != "test image" {
		t.Error("buildResponse() first image data mismatch")
	}
	if resp.Images[1].URL != "https://example.com/img.png" {
		t.Error("buildResponse() second image URL mismatch")
	}
}

func TestProvider_buildResponse_InvalidBase64(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	apiResp := apiResponse{
		Data: []imageData{
			{B64JSON: "not-valid-base64!!!"},
		},
	}

	_, err := p.buildResponse(apiResp)
	if err == nil {
		t.Fatal("buildResponse() error = nil, want error for invalid base64")
	}
}

func TestProvider_DownloadImage(t *testing.T) {
	expectedData := []byte("downloaded image content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(expectedData)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	data, err := p.DownloadImage(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("DownloadImage() error = %v", err)
	}
	if string(data) != string(expectedData) {
		t.Errorf("DownloadImage() data mismatch")
	}
}

func TestProvider_DownloadImage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	_, err := p.DownloadImage(context.Background(), server.URL)
	if err == nil {
		t.Fatal("DownloadImage() error = nil, want error")
	}
}

func TestProvider_Generate_MultipleImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img1"))},
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img2"))},
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img3"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  3,
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(resp.Images) != 3 {
		t.Errorf("Generate() returned %d images, want 3", len(resp.Images))
	}

	for i, img := range resp.Images {
		if img.Index != i {
			t.Errorf("Image %d has Index = %d", i, img.Index)
		}
	}
}

func TestProvider_Generate_InvalidBaseURL(t *testing.T) {
	cfg := &provider.Config{APIKey: "test-key", BaseURL: "http://[::1]:namedport"}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  1,
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("Generate() error = nil, want error for invalid URL")
	}
}

func TestProvider_DownloadImage_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("data"))
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.DownloadImage(ctx, server.URL)
	if err == nil {
		t.Fatal("DownloadImage() error = nil, want error for canceled context")
	}
}

func TestProvider_DownloadImage_InvalidURL(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	_, err := p.DownloadImage(context.Background(), "http://[::1]:namedport")
	if err == nil {
		t.Fatal("DownloadImage() error = nil, want error for invalid URL")
	}
}

func TestProvider_buildAPIRequest_EmptyFormat(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test",
		Count:  1,
		Format: "", // Empty format
	}

	apiReq := p.buildAPIRequest(req)
	if apiReq.OutputFormat != "" {
		t.Errorf("buildAPIRequest() OutputFormat should be empty")
	}
}

func TestProvider_buildAPIRequest_DallE3NoStyle(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:  "dall-e-3",
		Prompt: "test",
		Count:  1,
		Style:  "", // No style
	}

	apiReq := p.buildAPIRequest(req)
	if apiReq.Style != "" {
		t.Errorf("buildAPIRequest() Style should be empty")
	}
}

func TestProvider_buildAPIRequest_NoQuality(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:   "gpt-image-1",
		Prompt:  "test",
		Count:   1,
		Quality: "", // No quality
	}

	apiReq := p.buildAPIRequest(req)
	if apiReq.Quality != "" {
		t.Errorf("buildAPIRequest() Quality should be empty")
	}
}

func TestProvider_buildAPIRequest_NotTransparent(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:       "gpt-image-1",
		Prompt:      "test",
		Count:       1,
		Transparent: false,
	}

	apiReq := p.buildAPIRequest(req)
	if apiReq.Background != "" {
		t.Errorf("buildAPIRequest() Background should be empty")
	}
}

func TestProvider_buildResponse_NoRevisedPromptOnSecondImage(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	apiResp := apiResponse{
		Data: []imageData{
			{B64JSON: base64.StdEncoding.EncodeToString([]byte("img1"))},
			{B64JSON: base64.StdEncoding.EncodeToString([]byte("img2")), RevisedPrompt: "ignored"},
		},
	}

	resp, err := p.buildResponse(apiResp)
	if err != nil {
		t.Fatalf("buildResponse() error = %v", err)
	}

	// RevisedPrompt should come only from first image
	if resp.RevisedPrompt != "" {
		t.Errorf("buildResponse() should not have revised prompt from second image")
	}
}
