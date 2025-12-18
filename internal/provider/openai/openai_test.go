package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestProvider_Verbose_Enabled(t *testing.T) {
	cfg := &provider.Config{APIKey: "test-key", Verbose: true}
	p, err := New(cfg, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !p.verbose {
		t.Error("Provider verbose should be true")
	}
}

func TestProvider_Verbose_Disabled(t *testing.T) {
	cfg := &provider.Config{APIKey: "test-key", Verbose: false}
	p, err := New(cfg, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if p.verbose {
		t.Error("Provider verbose should be false")
	}
}

func TestProvider_logRequest_VerboseDisabled(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: false}, models.DefaultRegistry())

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	p.logRequest("POST", "https://api.openai.com/v1/images/generations", headers, []byte(`{"prompt":"test"}`))

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("logRequest() should not output when verbose is disabled, got: %s", output)
	}
}

func TestProvider_logRequest_VerboseEnabled(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Authorization", "Bearer sk-secret-key-12345")
	body := []byte(`{"prompt":"test prompt","model":"gpt-image-1"}`)
	p.logRequest("POST", "https://api.openai.com/v1/images/generations", headers, body)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check for expected content
	if !strings.Contains(output, "--- REQUEST ---") {
		t.Error("logRequest() output should contain '--- REQUEST ---'")
	}
	if !strings.Contains(output, "POST https://api.openai.com/v1/images/generations") {
		t.Error("logRequest() output should contain method and URL")
	}
	if !strings.Contains(output, "Headers:") {
		t.Error("logRequest() output should contain 'Headers:'")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("logRequest() output should redact Authorization header")
	}
	if strings.Contains(output, "sk-secret-key-12345") {
		t.Error("logRequest() output should NOT contain actual API key")
	}
	if !strings.Contains(output, "Body:") {
		t.Error("logRequest() output should contain 'Body:'")
	}
	if !strings.Contains(output, "test prompt") {
		t.Error("logRequest() output should contain request body content")
	}
}

func TestProvider_logRequest_EmptyBody(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	p.logRequest("GET", "https://api.openai.com/v1/test", headers, nil)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if strings.Contains(output, "Body:") {
		t.Error("logRequest() should not contain Body section for empty body")
	}
}

func TestProvider_logRequest_InvalidJSON(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	p.logRequest("POST", "https://api.openai.com/v1/test", headers, []byte("not json"))

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "not json") {
		t.Error("logRequest() should output raw body when JSON is invalid")
	}
}

func TestProvider_logResponse_VerboseDisabled(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: false}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	p.logResponse(200, headers, []byte(`{"data":[]}`))

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("logResponse() should not output when verbose is disabled, got: %s", output)
	}
}

func TestProvider_logResponse_VerboseEnabled(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	body := []byte(`{"created":123456,"data":[{"url":"https://example.com/image.png"}]}`)
	p.logResponse(200, headers, body)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "--- RESPONSE ---") {
		t.Error("logResponse() output should contain '--- RESPONSE ---'")
	}
	if !strings.Contains(output, "Status: 200") {
		t.Error("logResponse() output should contain status code")
	}
	if !strings.Contains(output, "Headers:") {
		t.Error("logResponse() output should contain 'Headers:'")
	}
	if !strings.Contains(output, "Body:") {
		t.Error("logResponse() output should contain 'Body:'")
	}
}

func TestProvider_logResponse_TruncatesBase64(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create a response with a large base64 field
	longBase64 := strings.Repeat("A", 200)
	body := []byte(`{"data":[{"b64_json":"` + longBase64 + `"}]}`)
	headers := http.Header{}
	p.logResponse(200, headers, body)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "[truncated]") {
		t.Error("logResponse() should truncate large base64 data")
	}
	if strings.Contains(output, longBase64) {
		t.Error("logResponse() should NOT contain full base64 data")
	}
}

func TestProvider_logResponse_EmptyBody(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	p.logResponse(204, headers, nil)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if strings.Contains(output, "Body:") {
		t.Error("logResponse() should not contain Body section for empty body")
	}
}

func TestProvider_logResponse_InvalidJSON(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	headers := http.Header{}
	p.logResponse(200, headers, []byte("not json"))

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "not json") {
		t.Error("logResponse() should output raw body when JSON is invalid")
	}
}

func TestProvider_logMultipartRequest_VerboseDisabled(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: false}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "test",
		Image:  []byte("fake image"),
	}
	headers := http.Header{}
	p.logMultipartRequest("POST", "https://api.openai.com/v1/images/edits", headers, req)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("logMultipartRequest() should not output when verbose is disabled, got: %s", output)
	}
}

func TestProvider_logMultipartRequest_VerboseEnabled(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit this image",
		Image:  []byte("fake image data"),
		Mask:   []byte("fake mask data"),
		Size:   "1024x1024",
		Count:  2,
		Format: models.FormatPNG,
	}
	headers := http.Header{}
	headers.Set("Content-Type", "multipart/form-data")
	headers.Set("Authorization", "Bearer sk-secret")
	p.logMultipartRequest("POST", "https://api.openai.com/v1/images/edits", headers, req)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "--- REQUEST ---") {
		t.Error("logMultipartRequest() output should contain '--- REQUEST ---'")
	}
	if !strings.Contains(output, "POST https://api.openai.com/v1/images/edits") {
		t.Error("logMultipartRequest() output should contain method and URL")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("logMultipartRequest() output should redact Authorization header")
	}
	if strings.Contains(output, "sk-secret") {
		t.Error("logMultipartRequest() output should NOT contain actual API key")
	}
	if !strings.Contains(output, "Body (multipart form):") {
		t.Error("logMultipartRequest() output should contain 'Body (multipart form):'")
	}
	if !strings.Contains(output, "model: gpt-image-1") {
		t.Error("logMultipartRequest() output should contain model")
	}
	if !strings.Contains(output, "prompt: edit this image") {
		t.Error("logMultipartRequest() output should contain prompt")
	}
	if !strings.Contains(output, "image: [15 bytes]") {
		t.Error("logMultipartRequest() output should contain image byte count")
	}
	if !strings.Contains(output, "mask: [14 bytes]") {
		t.Error("logMultipartRequest() output should contain mask byte count")
	}
	if !strings.Contains(output, "size: 1024x1024") {
		t.Error("logMultipartRequest() output should contain size")
	}
	if !strings.Contains(output, "n: 2") {
		t.Error("logMultipartRequest() output should contain count")
	}
	if !strings.Contains(output, "output_format: png") {
		t.Error("logMultipartRequest() output should contain format")
	}
}

func TestProvider_logMultipartRequest_MinimalFields(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test", Verbose: true}, models.DefaultRegistry())

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "test",
		Image:  []byte("img"),
	}
	headers := http.Header{}
	p.logMultipartRequest("POST", "https://api.openai.com/v1/images/edits", headers, req)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should not contain optional fields when not set
	if strings.Contains(output, "mask:") {
		t.Error("logMultipartRequest() should not contain mask when empty")
	}
	if strings.Contains(output, "size:") {
		t.Error("logMultipartRequest() should not contain size when empty")
	}
	if strings.Contains(output, "n:") {
		t.Error("logMultipartRequest() should not contain count when 0")
	}
	if strings.Contains(output, "output_format:") {
		t.Error("logMultipartRequest() should not contain format when empty")
	}
}

func TestTruncateBase64InJSON_ValidJSON(t *testing.T) {
	longBase64 := strings.Repeat("A", 200)
	input := []byte(`{"data":[{"b64_json":"` + longBase64 + `","url":"https://example.com"}]}`)

	result := truncateBase64InJSON(input)

	var data map[string]interface{}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("truncateBase64InJSON() returned invalid JSON: %v", err)
	}

	dataArr := data["data"].([]interface{})
	firstItem := dataArr[0].(map[string]interface{})
	b64Value := firstItem["b64_json"].(string)

	if !strings.HasSuffix(b64Value, "... [truncated]") {
		t.Error("truncateBase64InJSON() should truncate b64_json field")
	}
	if len(b64Value) > 120 {
		t.Error("truncateBase64InJSON() truncated value too long")
	}

	// URL should be preserved
	if firstItem["url"] != "https://example.com" {
		t.Error("truncateBase64InJSON() should preserve other fields")
	}
}

func TestTruncateBase64InJSON_InvalidJSON(t *testing.T) {
	input := []byte("not valid json")
	result := truncateBase64InJSON(input)

	if string(result) != string(input) {
		t.Error("truncateBase64InJSON() should return original input for invalid JSON")
	}
}

func TestTruncateBase64InJSON_ShortBase64(t *testing.T) {
	shortBase64 := "aGVsbG8=" // "hello" in base64
	input := []byte(`{"data":[{"b64_json":"` + shortBase64 + `"}]}`)

	result := truncateBase64InJSON(input)

	var data map[string]interface{}
	json.Unmarshal(result, &data)
	dataArr := data["data"].([]interface{})
	firstItem := dataArr[0].(map[string]interface{})
	b64Value := firstItem["b64_json"].(string)

	if b64Value != shortBase64 {
		t.Error("truncateBase64InJSON() should not truncate short base64 strings")
	}
}

func TestTruncateBase64InJSON_NestedObjects(t *testing.T) {
	longBase64 := strings.Repeat("B", 200)
	input := []byte(`{"outer":{"inner":{"b64_json":"` + longBase64 + `"}}}`)

	result := truncateBase64InJSON(input)

	var data map[string]interface{}
	json.Unmarshal(result, &data)
	outer := data["outer"].(map[string]interface{})
	inner := outer["inner"].(map[string]interface{})
	b64Value := inner["b64_json"].(string)

	if !strings.HasSuffix(b64Value, "... [truncated]") {
		t.Error("truncateBase64InJSON() should truncate nested b64_json fields")
	}
}

func TestTruncateBase64Fields_NonStringValues(t *testing.T) {
	data := map[string]interface{}{
		"number": 42,
		"bool":   true,
		"null":   nil,
		"array":  []interface{}{1, 2, 3},
	}

	// Should not panic
	truncateBase64Fields(data)

	if data["number"] != 42 {
		t.Error("truncateBase64Fields() should preserve number values")
	}
	if data["bool"] != true {
		t.Error("truncateBase64Fields() should preserve bool values")
	}
}

func TestProvider_Generate_WithVerbose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Created: time.Now().Unix(),
			Data: []imageData{
				{
					B64JSON:       base64.StdEncoding.EncodeToString([]byte("test")),
					RevisedPrompt: "revised",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL, Verbose: true}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "gpt-image-1",
		Prompt: "test prompt",
		Count:  1,
	}

	_, err := p.Generate(context.Background(), req)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(output, "--- REQUEST ---") {
		t.Error("Generate() with verbose should log request")
	}
	if !strings.Contains(output, "--- RESPONSE ---") {
		t.Error("Generate() with verbose should log response")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Generate() with verbose should redact API key")
	}
}

func TestProvider_SupportsEdit(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	tests := []struct {
		model string
		want  bool
	}{
		{"gpt-image-1", true},
		{"dall-e-2", true},
		{"dall-e-3", false},
		{"unknown-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := p.SupportsEdit(tt.model); got != tt.want {
				t.Errorf("SupportsEdit(%s) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestProvider_Edit_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Error("expected multipart/form-data content type")
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong authorization header")
		}

		resp := apiResponse{
			Created: time.Now().Unix(),
			Data: []imageData{
				{
					B64JSON: base64.StdEncoding.EncodeToString([]byte("edited image")),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit this",
		Image:  []byte("fake image data"),
	}

	resp, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	if len(resp.Images) != 1 {
		t.Errorf("Edit() returned %d images, want 1", len(resp.Images))
	}
	if string(resp.Images[0].Data) != "edited image" {
		t.Error("Edit() image data mismatch")
	}
}

func TestProvider_Edit_WithMask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
		Mask:   []byte("mask"),
	}

	_, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() with mask error = %v", err)
	}
}

func TestProvider_Edit_WithAllOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
		Size:   "1024x1024",
		Count:  2,
		Format: models.FormatPNG,
	}

	_, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() with all options error = %v", err)
	}
}

func TestProvider_Edit_DallE2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{{URL: "https://example.com/edited.png"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "dall-e-2",
		Prompt: "edit",
		Image:  []byte("image"),
	}

	resp, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() dall-e-2 error = %v", err)
	}

	if resp.Images[0].URL != "https://example.com/edited.png" {
		t.Error("Edit() URL mismatch")
	}
}

func TestProvider_Edit_ValidationError(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "",
		Image:  []byte("image"),
	}

	_, err := p.Edit(context.Background(), req)
	if err == nil {
		t.Fatal("Edit() should return error for invalid request")
	}
}

func TestProvider_Edit_UnsupportedModel(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "dall-e-3",
		Prompt: "edit",
		Image:  []byte("image"),
	}

	_, err := p.Edit(context.Background(), req)
	if err == nil {
		t.Fatal("Edit() should return error for unsupported model")
	}
	if !errors.Is(err, provider.ErrEditNotSupported) {
		t.Errorf("Edit() error = %v, want %v", err, provider.ErrEditNotSupported)
	}
}

func TestProvider_Edit_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Error: &apiError{
				Message: "invalid image",
				Type:    "invalid_request_error",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
	}

	_, err := p.Edit(context.Background(), req)
	if err == nil {
		t.Fatal("Edit() should return error for API error")
	}
	if !errors.Is(err, provider.ErrEditFailed) {
		t.Errorf("Edit() error = %v, want %v", err, provider.ErrEditFailed)
	}
}

func TestProvider_Edit_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(apiResponse{})
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
	}

	_, err := p.Edit(context.Background(), req)
	if err == nil {
		t.Fatal("Edit() should return error for HTTP error")
	}
}

func TestProvider_Edit_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
	}

	_, err := p.Edit(context.Background(), req)
	if err == nil {
		t.Fatal("Edit() should return error for invalid JSON")
	}
}

func TestProvider_Edit_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(apiResponse{})
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
	}

	_, err := p.Edit(ctx, req)
	if err == nil {
		t.Fatal("Edit() should return error for canceled context")
	}
}

func TestProvider_Edit_WithVerbose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL, Verbose: true}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit this image",
		Image:  []byte("fake image"),
	}

	_, err := p.Edit(context.Background(), req)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	if !strings.Contains(output, "--- REQUEST ---") {
		t.Error("Edit() with verbose should log request")
	}
	if !strings.Contains(output, "--- RESPONSE ---") {
		t.Error("Edit() with verbose should log response")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Edit() with verbose should redact API key")
	}
	if !strings.Contains(output, "multipart form") {
		t.Error("Edit() with verbose should show multipart form info")
	}
}

// Cost Integration Tests

func TestProvider_Generate_ReturnsCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Created: time.Now().Unix(),
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("test"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "gpt-image-1",
		Prompt:  "test prompt",
		Count:   1,
		Size:    "1024x1024",
		Quality: "medium",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if resp.Cost == nil {
		t.Fatal("Generate() should return cost information")
	}
	if resp.Cost.Total <= 0 {
		t.Errorf("Generate() cost.Total = %f, want > 0", resp.Cost.Total)
	}
	if resp.Cost.PerImage <= 0 {
		t.Errorf("Generate() cost.PerImage = %f, want > 0", resp.Cost.PerImage)
	}
	if resp.Cost.Currency != "USD" {
		t.Errorf("Generate() cost.Currency = %s, want USD", resp.Cost.Currency)
	}
}

func TestProvider_Generate_Cost_GPTImage1_Medium(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "gpt-image-1",
		Prompt:  "test",
		Count:   1,
		Size:    "1024x1024",
		Quality: "medium",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Medium quality 1024x1024 should cost $0.042
	expectedCost := 0.042
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_GPTImage1_High(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "gpt-image-1",
		Prompt:  "test",
		Count:   1,
		Size:    "1024x1024",
		Quality: "high",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// High quality 1024x1024 should cost $0.167
	expectedCost := 0.167
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_GPTImage1_Low(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "gpt-image-1",
		Prompt:  "test",
		Count:   1,
		Size:    "1024x1024",
		Quality: "low",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Low quality 1024x1024 should cost $0.011
	expectedCost := 0.011
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_GPTImage1_LargeSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("img"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "gpt-image-1",
		Prompt:  "test",
		Count:   1,
		Size:    "1536x1024",
		Quality: "high",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// High quality 1536x1024 should cost $0.250
	expectedCost := 0.250
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_DallE3_Standard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{URL: "https://example.com/image.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "dall-e-3",
		Prompt:  "test",
		Count:   1,
		Size:    "1024x1024",
		Quality: "standard",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// DALL-E 3 standard 1024x1024 should cost $0.040
	expectedCost := 0.040
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_DallE3_HD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{URL: "https://example.com/image.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:   "dall-e-3",
		Prompt:  "test",
		Count:   1,
		Size:    "1024x1024",
		Quality: "hd",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// DALL-E 3 hd 1024x1024 should cost $0.080
	expectedCost := 0.080
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_DallE2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{URL: "https://example.com/image.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.Request{
		Model:  "dall-e-2",
		Prompt: "test",
		Count:  1,
		Size:   "512x512",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// DALL-E 2 512x512 should cost $0.018
	expectedCost := 0.018
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Generate_Cost_MultipleImages(t *testing.T) {
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
		Model:   "gpt-image-1",
		Prompt:  "test",
		Count:   3,
		Size:    "1024x1024",
		Quality: "low",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// 3 images at $0.011 each = $0.033
	expectedPerImage := 0.011
	expectedTotal := 0.033
	if !floatEquals(resp.Cost.PerImage, expectedPerImage) {
		t.Errorf("Generate() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedPerImage)
	}
	if !floatEquals(resp.Cost.Total, expectedTotal) {
		t.Errorf("Generate() cost.Total = %f, want %f", resp.Cost.Total, expectedTotal)
	}
}

func TestProvider_Edit_ReturnsCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("edited"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit this",
		Image:  []byte("fake image"),
		Size:   "1024x1024",
	}

	resp, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	if resp.Cost == nil {
		t.Fatal("Edit() should return cost information")
	}
	if resp.Cost.Total <= 0 {
		t.Errorf("Edit() cost.Total = %f, want > 0", resp.Cost.Total)
	}
	if resp.Cost.Currency != "USD" {
		t.Errorf("Edit() cost.Currency = %s, want USD", resp.Cost.Currency)
	}
}

func TestProvider_Edit_Cost_GPTImage1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("edited"))},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "gpt-image-1",
		Prompt: "edit",
		Image:  []byte("image"),
		Size:   "1024x1024",
	}

	resp, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	// Edit uses medium quality, 1024x1024 = $0.042
	expectedCost := 0.042
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Edit() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_Edit_Cost_DallE2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Data: []imageData{
				{URL: "https://example.com/edited.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &provider.Config{APIKey: "test-key", BaseURL: server.URL}
	p, _ := New(cfg, models.DefaultRegistry())

	req := &models.EditRequest{
		Model:  "dall-e-2",
		Prompt: "edit",
		Image:  []byte("image"),
		Size:   "512x512",
	}

	resp, err := p.Edit(context.Background(), req)
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	// DALL-E 2 512x512 = $0.018
	expectedCost := 0.018
	if !floatEquals(resp.Cost.PerImage, expectedCost) {
		t.Errorf("Edit() cost.PerImage = %f, want %f", resp.Cost.PerImage, expectedCost)
	}
}

func TestProvider_NewHasCostCalculator(t *testing.T) {
	cfg := &provider.Config{APIKey: "test-key"}
	p, err := New(cfg, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if p.costCalc == nil {
		t.Error("New() should initialize cost calculator")
	}
}

func floatEquals(a, b float64) bool {
	const epsilon = 0.0001
	return (a-b) < epsilon && (b-a) < epsilon
}
