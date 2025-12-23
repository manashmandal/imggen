package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
