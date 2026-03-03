package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestGenerateWithResponsesNon200StatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{ID: "resp_err500"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}

	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}

func TestGenerateWithResponsesEmptyOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID:     "resp_empty",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("generate something")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "resp_empty" {
		t.Errorf("expected ID resp_empty, got %s", resp.ID)
	}

	if len(resp.Images) != 0 {
		t.Errorf("expected 0 images, got %d", len(resp.Images))
	}
}

func TestGenerateWithResponsesMultipleImages(t *testing.T) {
	img1 := []byte("image-one-data")
	img2 := []byte("image-two-data")
	img3 := []byte("image-three-data")
	b64Img1 := base64.StdEncoding.EncodeToString(img1)
	b64Img2 := base64.StdEncoding.EncodeToString(img2)
	b64Img3 := base64.StdEncoding.EncodeToString(img3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID: "resp_multi",
			Output: []responsesOutputItem{
				{Type: "image_generation_call", Status: "completed", Result: b64Img1, RevisedPrompt: "first prompt"},
				{Type: "image_generation_call", Status: "completed", Result: b64Img2, RevisedPrompt: "second prompt"},
				{Type: "image_generation_call", Status: "completed", Result: b64Img3},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("three images please")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(resp.Images))
	}

	if string(resp.Images[0].Data) != string(img1) {
		t.Error("image 0 data mismatch")
	}
	if string(resp.Images[1].Data) != string(img2) {
		t.Error("image 1 data mismatch")
	}
	if string(resp.Images[2].Data) != string(img3) {
		t.Error("image 2 data mismatch")
	}

	if resp.Images[0].Index != 0 || resp.Images[1].Index != 1 || resp.Images[2].Index != 2 {
		t.Errorf("unexpected indices: %d, %d, %d", resp.Images[0].Index, resp.Images[1].Index, resp.Images[2].Index)
	}

	if resp.RevisedPrompt != "second prompt" {
		t.Errorf("expected last revised prompt to win, got %q", resp.RevisedPrompt)
	}
}

func TestGenerateWithResponsesUsageCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID:     "resp_usage",
			Output: []responsesOutputItem{},
			Usage: &responsesUsage{
				InputTokens:  1000,
				OutputTokens: 500,
				TotalTokens:  1500,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test with usage")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Cost == nil {
		t.Fatal("expected cost info, got nil")
	}

	if resp.Cost.Total <= 0 {
		t.Errorf("expected positive total cost, got %f", resp.Cost.Total)
	}

	if resp.Cost.Currency != "USD" {
		t.Errorf("expected currency USD, got %s", resp.Cost.Currency)
	}
}

func TestGenerateWithResponsesNilUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID:     "resp_no_usage",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test without usage")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Cost != nil {
		t.Errorf("expected nil cost when no usage, got %+v", resp.Cost)
	}
}

func TestGenerateWithResponsesImagePaths(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "imggen-test-*.png")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if _, err := tmpFile.Write(pngHeader); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	var receivedInput json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&raw)
		receivedInput = raw["input"]

		resp := responsesAPIResponse{
			ID:     "resp_path",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("edit from file")
	req.Model = "gpt-image-1.5"
	req.ImagePaths = []string{tmpFile.Name()}

	_, err = p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var messages []responsesInputMessage
	if err := json.Unmarshal(receivedInput, &messages); err != nil {
		t.Fatalf("expected array input, got: %s", string(receivedInput))
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if len(messages[0].Content) != 2 {
		t.Fatalf("expected 2 content items (text + image), got %d", len(messages[0].Content))
	}

	if messages[0].Content[1].Type != "input_image" {
		t.Errorf("expected second content type 'input_image', got %q", messages[0].Content[1].Type)
	}

	if !strings.HasPrefix(messages[0].Content[1].ImageURL, "data:image/png;base64,") {
		t.Errorf("expected data URI with png mime type, got %q", messages[0].Content[1].ImageURL)
	}
}

func TestGenerateWithResponsesImagePathNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called when image path is invalid")
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("edit nonexistent")
	req.Model = "gpt-image-1.5"
	req.ImagePaths = []string{"/nonexistent/path/image.png"}

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for nonexistent image path, got nil")
	}

	if !strings.Contains(err.Error(), "failed to read image") {
		t.Errorf("expected 'failed to read image' error, got: %v", err)
	}
}

func TestGenerateWithResponsesImageURLs(t *testing.T) {
	var receivedInput json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&raw)
		receivedInput = raw["input"]

		resp := responsesAPIResponse{
			ID:     "resp_urls",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("edit from urls")
	req.Model = "gpt-image-1.5"
	req.ImageURLs = []string{
		"https://example.com/image1.png",
		"https://example.com/image2.jpg",
	}

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var messages []responsesInputMessage
	if err := json.Unmarshal(receivedInput, &messages); err != nil {
		t.Fatalf("expected array input, got: %s", string(receivedInput))
	}

	if len(messages[0].Content) != 3 {
		t.Fatalf("expected 3 content items (text + 2 images), got %d", len(messages[0].Content))
	}

	if messages[0].Content[1].ImageURL != "https://example.com/image1.png" {
		t.Errorf("expected first image URL passthrough, got %q", messages[0].Content[1].ImageURL)
	}

	if messages[0].Content[2].ImageURL != "https://example.com/image2.jpg" {
		t.Errorf("expected second image URL passthrough, got %q", messages[0].Content[2].ImageURL)
	}
}

func TestGenerateWithResponsesMultiImageInputTypes(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "imggen-test-multi-*.png")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	tmpFile.Write(pngHeader)
	tmpFile.Close()

	var receivedInput json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&raw)
		receivedInput = raw["input"]

		resp := responsesAPIResponse{
			ID:     "resp_mixed",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("combine all input types")
	req.Model = "gpt-image-1.5"
	req.ImagePaths = []string{tmpFile.Name()}
	req.ImageURLs = []string{"https://example.com/ref.png"}
	req.ImageData = [][]byte{{0xFF, 0xD8, 0xFF, 0xE0}}

	_, err = p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var messages []responsesInputMessage
	if err := json.Unmarshal(receivedInput, &messages); err != nil {
		t.Fatalf("expected array input, got: %s", string(receivedInput))
	}

	if len(messages[0].Content) != 4 {
		t.Fatalf("expected 4 content items (text + path + url + data), got %d", len(messages[0].Content))
	}

	if messages[0].Content[0].Type != "input_text" {
		t.Errorf("expected first content 'input_text', got %q", messages[0].Content[0].Type)
	}

	for i := 1; i <= 3; i++ {
		if messages[0].Content[i].Type != "input_image" {
			t.Errorf("expected content[%d] type 'input_image', got %q", i, messages[0].Content[i].Type)
		}
	}

	if !strings.HasPrefix(messages[0].Content[1].ImageURL, "data:image/png;base64,") {
		t.Errorf("expected path image as data URI with png, got: %q", messages[0].Content[1].ImageURL)
	}

	if messages[0].Content[2].ImageURL != "https://example.com/ref.png" {
		t.Errorf("expected URL passthrough, got %q", messages[0].Content[2].ImageURL)
	}

	if !strings.HasPrefix(messages[0].Content[3].ImageURL, "data:image/jpeg;base64,") {
		t.Errorf("expected inline data as data URI with jpeg, got: %q", messages[0].Content[3].ImageURL)
	}
}

func TestGenerateWithResponsesToolConfigAllFields(t *testing.T) {
	var receivedTools []responsesToolConfig

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesAPIRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedTools = req.Tools

		resp := responsesAPIResponse{
			ID:     "resp_allfields",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("full config")
	req.Model = "gpt-image-1.5"
	req.Action = "generate"
	req.Quality = "medium"
	req.Size = "1536x1024"
	req.Background = "opaque"
	req.InputFidelity = "high"
	req.OutputFormat = models.FormatWebP

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(receivedTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(receivedTools))
	}

	tool := receivedTools[0]

	checks := []struct {
		field    string
		got      string
		expected string
	}{
		{"Type", tool.Type, "image_generation"},
		{"Action", tool.Action, "generate"},
		{"Quality", tool.Quality, "medium"},
		{"Size", tool.Size, "1536x1024"},
		{"Background", tool.Background, "opaque"},
		{"InputFidelity", tool.InputFidelity, "high"},
		{"OutputFormat", tool.OutputFormat, "webp"},
	}

	for _, c := range checks {
		if c.got != c.expected {
			t.Errorf("expected %s %q, got %q", c.field, c.expected, c.got)
		}
	}
}

func TestGenerateWithResponsesToolConfigDefaults(t *testing.T) {
	var receivedTools []responsesToolConfig

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesAPIRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedTools = req.Tools

		resp := responsesAPIResponse{
			ID:     "resp_defaults",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := &models.ResponsesRequest{
		Prompt: "minimal config",
		Model:  "gpt-image-1.5",
	}

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tool := receivedTools[0]

	if tool.Type != "image_generation" {
		t.Errorf("expected type 'image_generation', got %q", tool.Type)
	}
	if tool.Action != "" {
		t.Errorf("expected empty action, got %q", tool.Action)
	}
	if tool.Quality != "" {
		t.Errorf("expected empty quality, got %q", tool.Quality)
	}
	if tool.Size != "" {
		t.Errorf("expected empty size, got %q", tool.Size)
	}
	if tool.Background != "" {
		t.Errorf("expected empty background, got %q", tool.Background)
	}
	if tool.InputFidelity != "" {
		t.Errorf("expected empty input_fidelity, got %q", tool.InputFidelity)
	}
	if tool.OutputFormat != "" {
		t.Errorf("expected empty output_format, got %q", tool.OutputFormat)
	}
}

func TestGenerateWithResponsesOutputSkipsNonImageItems(t *testing.T) {
	imgData := []byte("real-image")
	b64 := base64.StdEncoding.EncodeToString(imgData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID: "resp_mixed_output",
			Output: []responsesOutputItem{
				{Type: "text", Result: "some text"},
				{Type: "image_generation_call", Status: "completed", Result: b64, RevisedPrompt: "revised"},
				{Type: "image_generation_call", Status: "failed", Result: ""},
				{Type: "function_call", Result: "{}"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test mixed output")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Images) != 1 {
		t.Fatalf("expected 1 image (only completed with result), got %d", len(resp.Images))
	}

	if string(resp.Images[0].Data) != string(imgData) {
		t.Error("image data mismatch")
	}
}

func TestGenerateWithResponsesVerboseLogging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID:     "resp_verbose",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	p, _ := New(&provider.Config{
		APIKey:  "test-key-secret",
		BaseURL: server.URL,
		Verbose: true,
	}, models.DefaultRegistry())

	req := models.NewResponsesRequest("verbose test")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		w.Close()
		os.Stderr = oldStderr
		t.Fatalf("unexpected error: %v", err)
	}

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr
	output := buf.String()

	if !strings.Contains(output, "RESPONSES REQUEST") {
		t.Error("expected verbose output to contain 'RESPONSES REQUEST'")
	}

	if !strings.Contains(output, "POST") {
		t.Error("expected verbose output to contain HTTP method")
	}

	if !strings.Contains(output, "[REDACTED]") {
		t.Error("expected Authorization header to be redacted in verbose output")
	}

	if strings.Contains(output, "test-key-secret") {
		t.Error("API key should not appear in verbose output")
	}

	if !strings.Contains(output, "RESPONSE") {
		t.Error("expected verbose output to contain response logging")
	}
}

func TestGenerateWithResponsesVerboseDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID:     "resp_quiet",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	p, _ := New(&provider.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Verbose: false,
	}, models.DefaultRegistry())

	req := models.NewResponsesRequest("quiet test")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		w.Close()
		os.Stderr = oldStderr
		t.Fatalf("unexpected error: %v", err)
	}

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr
	output := buf.String()

	if strings.Contains(output, "RESPONSES REQUEST") {
		t.Error("verbose output should not appear when Verbose is false")
	}
}

func TestGenerateWithResponsesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json {{{"))
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestGenerateWithResponsesBase64Fields(t *testing.T) {
	imgData := []byte("test-image-bytes")
	b64 := base64.StdEncoding.EncodeToString(imgData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := responsesAPIResponse{
			ID: "resp_b64",
			Output: []responsesOutputItem{
				{Type: "image_generation_call", Status: "completed", Result: b64},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test base64")
	req.Model = "gpt-image-1.5"

	resp, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Images[0].Base64 != b64 {
		t.Error("expected Base64 field to be preserved in response")
	}

	if !bytes.Equal(resp.Images[0].Data, imgData) {
		t.Error("expected decoded Data to match original bytes")
	}
}

func TestGenerateWithResponsesRequestHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header

		resp := responsesAPIResponse{
			ID:     "resp_headers",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "my-api-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("test headers")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", receivedHeaders.Get("Content-Type"))
	}

	if receivedHeaders.Get("Authorization") != "Bearer my-api-key" {
		t.Errorf("expected Authorization 'Bearer my-api-key', got %q", receivedHeaders.Get("Authorization"))
	}
}

func TestGenerateWithResponsesContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := models.NewResponsesRequest("cancelled")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(ctx, req)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestGenerateWithResponsesTextOnlyInputIsString(t *testing.T) {
	var receivedBody map[string]json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)

		resp := responsesAPIResponse{
			ID:     "resp_string",
			Output: []responsesOutputItem{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := New(&provider.Config{APIKey: "test-key", BaseURL: server.URL}, models.DefaultRegistry())

	req := models.NewResponsesRequest("just text, no images")
	req.Model = "gpt-image-1.5"

	_, err := p.GenerateWithResponses(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var inputStr string
	if err := json.Unmarshal(receivedBody["input"], &inputStr); err != nil {
		t.Fatalf("expected input to be a plain string, got: %s", string(receivedBody["input"]))
	}

	if inputStr != "just text, no images" {
		t.Errorf("expected input string 'just text, no images', got %q", inputStr)
	}
}
