package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/manash/imggen/pkg/models"
)

type chatMessage struct {
	Role    string        `json:"role"`
	Content []chatContent `json:"content"`
}

type chatContent struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type chatRequest struct {
	Model               string            `json:"model"`
	Messages            []chatMessage     `json:"messages"`
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"`
	ResponseFormat      *responseFormat   `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type chatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   *chatUsage     `json:"usage,omitempty"`
	Error   *apiError      `json:"error,omitempty"`
}

type chatChoice struct {
	Index        int            `json:"index"`
	Message      chatMessageOut `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type chatMessageOut struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (p *Provider) SupportsOCR(model string) bool {
	cap, ok := p.registry.GetOCR(model)
	if !ok {
		return false
	}
	return cap.Provider == models.ProviderOpenAI
}

func (p *Provider) ListOCRModels() []string {
	return p.registry.ListOCRByProvider(models.ProviderOpenAI)
}

func (p *Provider) OCR(ctx context.Context, req *models.OCRRequest) (*models.OCRResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	imageContent, err := p.prepareImageContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare image: %w", err)
	}

	prompt := req.Prompt
	if prompt == "" {
		if len(req.Schema) > 0 {
			prompt = "Extract all text and data from this image and return it in the specified JSON structure."
		} else {
			prompt = "Extract all text from this image. Preserve the original formatting and structure as much as possible."
		}
	}

	messages := []chatMessage{
		{
			Role: "user",
			Content: []chatContent{
				{Type: "text", Text: prompt},
				imageContent,
			},
		},
	}

	chatReq := &chatRequest{
		Model:               req.Model,
		Messages:            messages,
		MaxCompletionTokens: req.MaxTokens,
	}

	if len(req.Schema) > 0 {
		schemaName := req.SchemaName
		if schemaName == "" {
			schemaName = "extracted_data"
		}
		chatReq.ResponseFormat = &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchema{
				Name:   schemaName,
				Strict: true,
				Schema: req.Schema,
			},
		}
	}

	jsonData, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logOCRRequest(http.MethodPost, url, httpReq.Header, chatReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	p.logResponse(resp.StatusCode, resp.Header, body)

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("OCR failed: %s", chatResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OCR failed: status %d", resp.StatusCode)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("OCR failed: no response choices")
	}

	content := chatResp.Choices[0].Message.Content

	ocrResp := &models.OCRResponse{}

	if len(req.Schema) > 0 {
		ocrResp.Structured = json.RawMessage(content)
	} else {
		ocrResp.Text = content
	}

	if chatResp.Usage != nil {
		ocrResp.InputTokens = chatResp.Usage.PromptTokens
		ocrResp.OutputTokens = chatResp.Usage.CompletionTokens
		ocrResp.Cost = p.costCalc.CalculateOCR(req.Model, chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens)
	}

	return ocrResp, nil
}

func (p *Provider) SuggestSchema(ctx context.Context, req *models.OCRRequest) (json.RawMessage, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	imageContent, err := p.prepareImageContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare image: %w", err)
	}

	prompt := `Analyze this image and suggest a JSON schema that would best capture its structured data.

Return ONLY a valid JSON schema object (no markdown, no explanation) that follows these rules:
1. Use JSON Schema draft-07 format
2. Include "type": "object" at the root
3. Add "properties" for each field you identify
4. Use appropriate types (string, number, integer, boolean, array, object)
5. Add "required" array for mandatory fields
6. Include "additionalProperties": false

Example format:
{
  "type": "object",
  "properties": {
    "field1": {"type": "string"},
    "field2": {"type": "number"}
  },
  "required": ["field1"],
  "additionalProperties": false
}`

	messages := []chatMessage{
		{
			Role: "user",
			Content: []chatContent{
				{Type: "text", Text: prompt},
				imageContent,
			},
		},
	}

	chatReq := &chatRequest{
		Model:               req.Model,
		Messages:            messages,
		MaxCompletionTokens: 2048,
	}

	jsonData, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logOCRRequest(http.MethodPost, url, httpReq.Header, chatReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	p.logResponse(resp.StatusCode, resp.Header, body)

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("schema suggestion failed: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("schema suggestion failed: no response choices")
	}

	content := chatResp.Choices[0].Message.Content
	content = strings.TrimSpace(content)

	// Remove markdown code fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Validate it's valid JSON
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(content), &schema); err != nil {
		return nil, fmt.Errorf("invalid schema returned: %w", err)
	}

	return json.RawMessage(content), nil
}

func (p *Provider) prepareImageContent(ctx context.Context, req *models.OCRRequest) (chatContent, error) {
	var imageData []byte
	var mimeType string

	if len(req.ImageData) > 0 {
		imageData = req.ImageData
		mimeType = detectMimeType(imageData)
	} else if req.ImagePath != "" {
		data, err := os.ReadFile(req.ImagePath)
		if err != nil {
			return chatContent{}, fmt.Errorf("failed to read image file: %w", err)
		}
		imageData = data
		mimeType = detectMimeType(data)
	} else if req.ImageURL != "" {
		return chatContent{
			Type: "image_url",
			ImageURL: &imageURL{
				URL:    req.ImageURL,
				Detail: "high",
			},
		}, nil
	}

	base64Data := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

	return chatContent{
		Type: "image_url",
		ImageURL: &imageURL{
			URL:    dataURL,
			Detail: "high",
		},
	}, nil
}

func detectMimeType(data []byte) string {
	if len(data) < 4 {
		return "application/octet-stream"
	}

	// Check magic bytes
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "image/gif"
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 4 && string(data[0:4]) == "%PDF" {
		return "application/pdf"
	}

	return "image/png" // Default to PNG
}

func (p *Provider) logOCRRequest(method, url string, headers http.Header, req *chatRequest) {
	if !p.verbose {
		return
	}

	fmt.Fprintln(os.Stderr, "--- OCR REQUEST ---")
	fmt.Fprintf(os.Stderr, "%s %s\n", method, url)
	fmt.Fprintln(os.Stderr, "Headers:")
	for key, values := range headers {
		for _, value := range values {
			if strings.ToLower(key) == "authorization" {
				value = "[REDACTED]"
			}
			fmt.Fprintf(os.Stderr, "  %s: %s\n", key, value)
		}
	}
	fmt.Fprintln(os.Stderr, "Body:")
	fmt.Fprintf(os.Stderr, "  model: %s\n", req.Model)
	fmt.Fprintf(os.Stderr, "  max_completion_tokens: %d\n", req.MaxCompletionTokens)
	if req.ResponseFormat != nil {
		fmt.Fprintf(os.Stderr, "  response_format: %s\n", req.ResponseFormat.Type)
	}
	fmt.Fprintln(os.Stderr, "  messages: [image content truncated]")
	fmt.Fprintln(os.Stderr, "-------------------")
}
