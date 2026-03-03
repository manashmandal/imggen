package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestGenerateWithResponses(t *testing.T) {
	testImageData := []byte("fake-image-data")
	b64Image := base64.StdEncoding.EncodeToString(testImageData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req responsesAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		resp := responsesAPIResponse{
			ID: "resp_test123",
			Output: []responsesOutputItem{
				{
					Type:          "image_generation_call",
					Status:        "completed",
					Action:        "generate",
					Result:        b64Image,
					RevisedPrompt: "A beautiful sunset",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, err := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	req := models.NewResponsesRequest("a sunset over mountains")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateWithResponses failed: %v", err)
	}

	if resp.ID != "resp_test123" {
		t.Errorf("expected ID resp_test123, got %s", resp.ID)
	}

	if len(resp.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(resp.Images))
	}

	if string(resp.Images[0].Data) != string(testImageData) {
		t.Error("image data mismatch")
	}

	if resp.RevisedPrompt != "A beautiful sunset" {
		t.Errorf("expected revised prompt 'A beautiful sunset', got %q", resp.RevisedPrompt)
	}
}

func TestGenerateWithResponsesPreviousID(t *testing.T) {
	var receivedPrevID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesAPIRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedPrevID = req.PreviousResponseID

		resp := responsesAPIResponse{
			ID:     "resp_follow123",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("make it brighter")
	req.Model = "gpt-image-1.5"
	req.PreviousResponseID = "resp_original123"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPrevID != "resp_original123" {
		t.Errorf("expected previous_response_id 'resp_original123', got %q", receivedPrevID)
	}
}

func TestGenerateWithResponsesWithImages(t *testing.T) {
	var receivedInput json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&raw)
		receivedInput = raw["input"]

		resp := responsesAPIResponse{
			ID:     "resp_img123",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("edit this image")
	req.Model = "gpt-image-1.5"
	req.ImageData = [][]byte{[]byte("fake-png-data")}

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var messages []responsesInputMessage
	if err := json.Unmarshal(receivedInput, &messages); err != nil {
		t.Fatalf("expected input to be array of messages, got: %s", string(receivedInput))
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if len(messages[0].Content) != 2 {
		t.Fatalf("expected 2 content items (text + image), got %d", len(messages[0].Content))
	}

	if messages[0].Content[0].Type != "input_text" {
		t.Errorf("expected first content type 'input_text', got %q", messages[0].Content[0].Type)
	}

	if messages[0].Content[1].Type != "input_image" {
		t.Errorf("expected second content type 'input_image', got %q", messages[0].Content[1].Type)
	}
}

func TestGenerateWithResponsesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			Error: &apiError{
				Message: "invalid model",
				Type:    "invalid_request_error",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "responses API error: invalid model" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGenerateWithResponsesToolConfig(t *testing.T) {
	var receivedTools []responsesToolConfig

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesAPIRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedTools = req.Tools

		resp := responsesAPIResponse{
			ID:     "resp_tool123",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("remove background")
	req.Model = "gpt-image-1.5"
	req.Action = "edit"
	req.Background = "transparent"
	req.Quality = "high"
	req.Size = "1024x1024"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(receivedTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(receivedTools))
	}

	tool := receivedTools[0]
	if tool.Type != "image_generation" {
		t.Errorf("expected tool type 'image_generation', got %q", tool.Type)
	}
	if tool.Action != "edit" {
		t.Errorf("expected action 'edit', got %q", tool.Action)
	}
	if tool.Background != "transparent" {
		t.Errorf("expected background 'transparent', got %q", tool.Background)
	}
	if tool.Quality != "high" {
		t.Errorf("expected quality 'high', got %q", tool.Quality)
	}
	if tool.Size != "1024x1024" {
		t.Errorf("expected size '1024x1024', got %q", tool.Size)
	}
}
