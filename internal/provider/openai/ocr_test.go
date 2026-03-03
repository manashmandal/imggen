package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestProvider_SupportsOCR(t *testing.T) {
	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key"}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	tests := []struct {
		model    string
		expected bool
	}{
		{"gpt-5.2", true},
		{"gpt-5-mini", true},
		{"gpt-5-nano", true},
		{"dall-e-3", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := prov.SupportsOCR(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsOCR(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestProvider_ListOCRModels(t *testing.T) {
	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key"}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	models := prov.ListOCRModels()
	if len(models) < 3 {
		t.Errorf("ListOCRModels() returned %d models, want at least 3", len(models))
	}

	// Check that known models are in the list
	found52 := false
	foundMini := false
	foundNano := false
	for _, m := range models {
		if m == "gpt-5.2" {
			found52 = true
		}
		if m == "gpt-5-mini" {
			foundMini = true
		}
		if m == "gpt-5-nano" {
			foundNano = true
		}
	}
	if !found52 {
		t.Error("ListOCRModels() should include gpt-5.2")
	}
	if !foundMini {
		t.Error("ListOCRModels() should include gpt-5-mini")
	}
	if !foundNano {
		t.Error("ListOCRModels() should include gpt-5-nano")
	}
}

func TestProvider_OCR_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("Expected path /chat/completions, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		resp := chatResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "gpt-5-mini",
			Choices: []chatChoice{
				{
					Index: 0,
					Message: chatMessageOut{
						Role:    "assistant",
						Content: "Extracted text from image",
					},
					FinishReason: "stop",
				},
			},
			Usage: &chatUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if resp.Text != "Extracted text from image" {
		t.Errorf("OCR().Text = %q, want %q", resp.Text, "Extracted text from image")
	}

	if resp.InputTokens != 100 {
		t.Errorf("OCR().InputTokens = %d, want 100", resp.InputTokens)
	}

	if resp.OutputTokens != 50 {
		t.Errorf("OCR().OutputTokens = %d, want 50", resp.OutputTokens)
	}

	if resp.Cost == nil {
		t.Error("OCR().Cost should not be nil")
	}
}

func TestProvider_OCR_WithSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		if req.ResponseFormat == nil {
			t.Error("Expected response_format to be set")
		}
		if req.ResponseFormat.Type != "json_schema" {
			t.Errorf("Expected response_format.type = json_schema, got %s", req.ResponseFormat.Type)
		}

		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessageOut{
						Content: `{"name": "John", "age": 30}`,
					},
				},
			},
			Usage: &chatUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["name", "age"],
		"additionalProperties": false
	}`)

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}
	req.Schema = schema

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if len(resp.Structured) == 0 {
		t.Error("OCR().Structured should not be empty")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Structured, &result); err != nil {
		t.Fatalf("Failed to unmarshal structured response: %v", err)
	}

	if result["name"] != "John" {
		t.Errorf("Expected name = John, got %v", result["name"])
	}
}

func TestProvider_OCR_ValidationError(t *testing.T) {
	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key"}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := &models.OCRRequest{} // Empty request

	_, err = prov.OCR(context.Background(), req)
	if err == nil {
		t.Error("OCR() should return error for empty request")
	}
}

func TestProvider_OCR_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Error: &apiError{
				Message: "Invalid API key",
				Type:    "invalid_request_error",
				Code:    "invalid_api_key",
			},
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{
		APIKey:  "invalid-key",
		BaseURL: server.URL,
	}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	_, err = prov.OCR(context.Background(), req)
	if err == nil {
		t.Error("OCR() should return error for API error response")
	}
}

func TestProvider_SuggestSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessageOut{
						Content: `{"type": "object", "properties": {"title": {"type": "string"}}, "required": ["title"], "additionalProperties": false}`,
					},
				},
			},
			Usage: &chatUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	schema, err := prov.SuggestSchema(context.Background(), req)
	if err != nil {
		t.Fatalf("SuggestSchema() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(schema, &result); err != nil {
		t.Fatalf("Failed to unmarshal schema: %v", err)
	}

	if result["type"] != "object" {
		t.Errorf("Expected type = object, got %v", result["type"])
	}
}

func TestProvider_OCR_WithURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessageOut{
						Content: "Text from URL image",
					},
				},
			},
			Usage: &chatUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageURL = "https://example.com/image.png"

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if resp.Text != "Text from URL image" {
		t.Errorf("OCR().Text = %q, want %q", resp.Text, "Text from URL image")
	}
}

func TestProvider_OCR_WithCustomPrompt(t *testing.T) {
	var receivedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 && len(req.Messages[0].Content) > 0 {
			receivedPrompt = req.Messages[0].Content[0].Text
		}

		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: "result"}}},
			Usage:   &chatUsage{PromptTokens: 10, CompletionTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}
	req.Prompt = "Extract only the title"

	_, err = prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if receivedPrompt != "Extract only the title" {
		t.Errorf("Expected custom prompt, got %q", receivedPrompt)
	}
}

func TestProvider_OCR_NoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{},
			Usage:   &chatUsage{PromptTokens: 10, CompletionTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	_, err = prov.OCR(context.Background(), req)
	if err == nil {
		t.Error("OCR() should return error when no choices returned")
	}
}

func TestProvider_OCR_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(chatResponse{})
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	_, err = prov.OCR(context.Background(), req)
	if err == nil {
		t.Error("OCR() should return error for HTTP 500")
	}
}

func TestProvider_OCR_WithSchemaName(t *testing.T) {
	var receivedSchemaName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat != nil && req.ResponseFormat.JSONSchema != nil {
			receivedSchemaName = req.ResponseFormat.JSONSchema.Name
		}

		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: `{"test": "value"}`}}},
			Usage:   &chatUsage{PromptTokens: 10, CompletionTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}
	req.Schema = json.RawMessage(`{"type": "object"}`)
	req.SchemaName = "my_custom_schema"

	_, err = prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if receivedSchemaName != "my_custom_schema" {
		t.Errorf("Expected schema name 'my_custom_schema', got %q", receivedSchemaName)
	}
}

func TestProvider_SuggestSchema_WithMarkdownFences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessageOut{
						Content: "```json\n{\"type\": \"object\", \"properties\": {}}\n```",
					},
				},
			},
			Usage: &chatUsage{PromptTokens: 100, CompletionTokens: 50},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	schema, err := prov.SuggestSchema(context.Background(), req)
	if err != nil {
		t.Fatalf("SuggestSchema() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(schema, &result); err != nil {
		t.Fatalf("Failed to unmarshal schema: %v", err)
	}

	if result["type"] != "object" {
		t.Errorf("Expected type = object, got %v", result["type"])
	}
}

func TestProvider_SuggestSchema_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{
				{
					Message: chatMessageOut{
						Content: "This is not valid JSON",
					},
				},
			},
			Usage: &chatUsage{PromptTokens: 100, CompletionTokens: 50},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	_, err = prov.SuggestSchema(context.Background(), req)
	if err == nil {
		t.Error("SuggestSchema() should return error for invalid JSON")
	}
}

func TestProvider_SuggestSchema_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Error: &apiError{Message: "Rate limit exceeded"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	_, err = prov.SuggestSchema(context.Background(), req)
	if err == nil {
		t.Error("SuggestSchema() should return error for API error")
	}
}

func TestProvider_SuggestSchema_NoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{},
			Usage:   &chatUsage{PromptTokens: 100, CompletionTokens: 50},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}

	_, err = prov.SuggestSchema(context.Background(), req)
	if err == nil {
		t.Error("SuggestSchema() should return error when no choices returned")
	}
}

func TestProvider_OCR_Verbose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: "test"}}},
			Usage:   &chatUsage{PromptTokens: 10, CompletionTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Verbose: true,
	}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageData = []byte{0x89, 0x50, 0x4E, 0x47}
	req.Schema = json.RawMessage(`{"type": "object"}`)

	_, err = prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}
}

func TestProvider_OCR_WithFilePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: "Text from file"}}},
			Usage:   &chatUsage{PromptTokens: 100, CompletionTokens: 50},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create a temp file with PNG magic bytes
	tmpFile, err := os.CreateTemp("", "test-image-*.png")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	tmpFile.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImagePath = tmpFile.Name()

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if resp.Text != "Text from file" {
		t.Errorf("OCR().Text = %q, want %q", resp.Text, "Text from file")
	}
}

func TestProvider_OCR_FileNotFound(t *testing.T) {
	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key"}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImagePath = "/nonexistent/path/to/image.png"

	_, err = prov.OCR(context.Background(), req)
	if err == nil {
		t.Error("OCR() should return error for nonexistent file")
	}
}

func TestProvider_SuggestSchema_ValidationError(t *testing.T) {
	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key"}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := &models.OCRRequest{} // Empty request

	_, err = prov.SuggestSchema(context.Background(), req)
	if err == nil {
		t.Error("SuggestSchema() should return error for empty request")
	}
}

func createTempPNG(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-ocr-*.png")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	// PNG magic bytes followed by minimal data
	f.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestProvider_OCR_MultipleImagePaths(t *testing.T) {
	path1 := createTempPNG(t)
	path2 := createTempPNG(t)

	var receivedReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: "multi image result"}}},
			Usage:   &chatUsage{PromptTokens: 200, CompletionTokens: 100},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImagePaths = []string{path1, path2}

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if resp.Text != "multi image result" {
		t.Errorf("OCR().Text = %q, want %q", resp.Text, "multi image result")
	}

	content := receivedReq.Messages[0].Content
	// Expect: 1 text entry + 2 image entries from ImagePaths
	if len(content) != 3 {
		t.Fatalf("Expected 3 content entries (1 text + 2 images), got %d", len(content))
	}
	if content[0].Type != "text" {
		t.Errorf("content[0].Type = %q, want %q", content[0].Type, "text")
	}
	for i := 1; i <= 2; i++ {
		if content[i].Type != "image_url" {
			t.Errorf("content[%d].Type = %q, want %q", i, content[i].Type, "image_url")
		}
		if content[i].ImageURL == nil {
			t.Fatalf("content[%d].ImageURL is nil", i)
		}
		if !strings.HasPrefix(content[i].ImageURL.URL, "data:image/png;base64,") {
			t.Errorf("content[%d].ImageURL.URL does not start with data:image/png;base64,", i)
		}
	}
}

func TestProvider_OCR_MultipleImageURLs(t *testing.T) {
	var receivedReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: "urls result"}}},
			Usage:   &chatUsage{PromptTokens: 200, CompletionTokens: 100},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImageURLs = []string{
		"https://example.com/img1.png",
		"https://example.com/img2.png",
		"https://example.com/img3.png",
	}

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if resp.Text != "urls result" {
		t.Errorf("OCR().Text = %q, want %q", resp.Text, "urls result")
	}

	content := receivedReq.Messages[0].Content
	// Expect: 1 text entry + 3 image_url entries
	if len(content) != 4 {
		t.Fatalf("Expected 4 content entries (1 text + 3 urls), got %d", len(content))
	}
	if content[0].Type != "text" {
		t.Errorf("content[0].Type = %q, want %q", content[0].Type, "text")
	}

	expectedURLs := []string{
		"https://example.com/img1.png",
		"https://example.com/img2.png",
		"https://example.com/img3.png",
	}
	for i, expected := range expectedURLs {
		idx := i + 1
		if content[idx].Type != "image_url" {
			t.Errorf("content[%d].Type = %q, want %q", idx, content[idx].Type, "image_url")
		}
		if content[idx].ImageURL == nil {
			t.Fatalf("content[%d].ImageURL is nil", idx)
		}
		if content[idx].ImageURL.URL != expected {
			t.Errorf("content[%d].ImageURL.URL = %q, want %q", idx, content[idx].ImageURL.URL, expected)
		}
		if content[idx].ImageURL.Detail != "high" {
			t.Errorf("content[%d].ImageURL.Detail = %q, want %q", idx, content[idx].ImageURL.Detail, "high")
		}
	}
}

func TestProvider_OCR_MixedImagePathsAndURLs(t *testing.T) {
	path1 := createTempPNG(t)

	var receivedReq chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedReq)
		resp := chatResponse{
			Choices: []chatChoice{{Message: chatMessageOut{Content: "mixed result"}}},
			Usage:   &chatUsage{PromptTokens: 300, CompletionTokens: 150},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImagePaths = []string{path1}
	req.ImageURLs = []string{"https://example.com/remote.png"}

	resp, err := prov.OCR(context.Background(), req)
	if err != nil {
		t.Fatalf("OCR() error = %v", err)
	}

	if resp.Text != "mixed result" {
		t.Errorf("OCR().Text = %q, want %q", resp.Text, "mixed result")
	}

	content := receivedReq.Messages[0].Content
	// Expect: 1 text + 1 file-based image + 1 URL-based image
	if len(content) != 3 {
		t.Fatalf("Expected 3 content entries (1 text + 1 file + 1 url), got %d", len(content))
	}

	if content[0].Type != "text" {
		t.Errorf("content[0].Type = %q, want %q", content[0].Type, "text")
	}

	// File-based image comes first (from ImagePaths)
	if content[1].Type != "image_url" {
		t.Errorf("content[1].Type = %q, want %q", content[1].Type, "image_url")
	}
	if content[1].ImageURL == nil {
		t.Fatal("content[1].ImageURL is nil")
	}
	if !strings.HasPrefix(content[1].ImageURL.URL, "data:image/png;base64,") {
		t.Error("content[1] should be a base64 data URL from file")
	}

	// URL-based image comes second (from ImageURLs)
	if content[2].Type != "image_url" {
		t.Errorf("content[2].Type = %q, want %q", content[2].Type, "image_url")
	}
	if content[2].ImageURL == nil {
		t.Fatal("content[2].ImageURL is nil")
	}
	if content[2].ImageURL.URL != "https://example.com/remote.png" {
		t.Errorf("content[2].ImageURL.URL = %q, want %q", content[2].ImageURL.URL, "https://example.com/remote.png")
	}
}

func TestProvider_OCR_MultipleImagePaths_FileNotFound(t *testing.T) {
	validPath := createTempPNG(t)

	registry := models.DefaultRegistry()
	prov, err := New(&provider.Config{APIKey: "test-key"}, registry)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	req := models.NewOCRRequest()
	req.ImagePaths = []string{validPath, "/nonexistent/image.png"}

	_, err = prov.OCR(context.Background(), req)
	if err == nil {
		t.Error("OCR() should return error when an ImagePaths entry does not exist")
	}
	if !strings.Contains(err.Error(), "failed to prepare image") {
		t.Errorf("error should mention 'failed to prepare image', got: %v", err)
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "PNG",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expected: "image/png",
		},
		{
			name:     "JPEG",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0},
			expected: "image/jpeg",
		},
		{
			name:     "GIF",
			data:     []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61},
			expected: "image/gif",
		},
		{
			name:     "WEBP",
			data:     []byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'},
			expected: "image/webp",
		},
		{
			name:     "PDF",
			data:     []byte{'%', 'P', 'D', 'F', '-', '1', '.', '4'},
			expected: "application/pdf",
		},
		{
			name:     "unknown",
			data:     []byte{0x00, 0x00, 0x00, 0x00},
			expected: "image/png", // defaults to PNG
		},
		{
			name:     "too short",
			data:     []byte{0x89},
			expected: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectMimeType(tt.data)
			if result != tt.expected {
				t.Errorf("detectMimeType() = %q, want %q", result, tt.expected)
			}
		})
	}
}
