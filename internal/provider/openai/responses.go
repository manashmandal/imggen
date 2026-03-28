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

	"github.com/manash/imggen/pkg/models"
)

type responsesAPIRequest struct {
	Model              string                `json:"model"`
	Input              interface{}           `json:"input"`
	Tools              []responsesToolConfig `json:"tools"`
	PreviousResponseID string                `json:"previous_response_id,omitempty"`
}

type responsesToolConfig struct {
	Type              string `json:"type"`
	Action            string `json:"action,omitempty"`
	InputFidelity     string `json:"input_fidelity,omitempty"`
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Size              string `json:"size,omitempty"`
	Model             string `json:"model,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
}

type responsesInputMessage struct {
	Role    string                 `json:"role"`
	Content []responsesContentItem `json:"content"`
}

type responsesContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesAPIResponse struct {
	ID     string                `json:"id"`
	Output []responsesOutputItem `json:"output"`
	Usage  *responsesUsage       `json:"usage,omitempty"`
	Error  *apiError             `json:"error,omitempty"`
}

type responsesOutputItem struct {
	Type          string `json:"type"`
	ID            string `json:"id,omitempty"`
	Status        string `json:"status,omitempty"`
	Action        string `json:"action,omitempty"`
	Result        string `json:"result,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	Background    string `json:"background,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (p *Provider) GenerateWithResponses(ctx context.Context, req *models.ResponsesRequest) (*models.ResponsesResponse, error) {
	var input interface{}

	if len(req.ImagePaths) > 0 || len(req.ImageURLs) > 0 || len(req.ImageData) > 0 {
		content := []responsesContentItem{
			{Type: "input_text", Text: req.Prompt},
		}
		for _, path := range req.ImagePaths {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("failed to read image %s: %w", path, err)
			}
			mimeType := detectMimeType(data)
			b64 := base64.StdEncoding.EncodeToString(data)
			dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
			content = append(content, responsesContentItem{
				Type:     "input_image",
				ImageURL: dataURL,
			})
		}
		for _, url := range req.ImageURLs {
			content = append(content, responsesContentItem{
				Type:     "input_image",
				ImageURL: url,
			})
		}
		for _, data := range req.ImageData {
			mimeType := detectMimeType(data)
			b64 := base64.StdEncoding.EncodeToString(data)
			dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
			content = append(content, responsesContentItem{
				Type:     "input_image",
				ImageURL: dataURL,
			})
		}
		input = []responsesInputMessage{{Role: "user", Content: content}}
	} else {
		input = req.Prompt
	}

	tool := responsesToolConfig{
		Type: "image_generation",
	}
	if req.Action != "" {
		tool.Action = req.Action
	}
	if req.InputFidelity != "" {
		tool.InputFidelity = req.InputFidelity
	}
	if req.Background != "" {
		tool.Background = req.Background
	}
	if req.OutputFormat != "" {
		tool.OutputFormat = req.OutputFormat.String()
	}
	if req.Quality != "" {
		tool.Quality = req.Quality
	}
	if req.Size != "" {
		tool.Size = req.Size
	}
	if req.ImageModel != "" {
		tool.Model = req.ImageModel
	}
	if req.OutputCompression != nil {
		tool.OutputCompression = req.OutputCompression
	}
	if req.Moderation != "" {
		tool.Moderation = req.Moderation
	}

	apiReq := &responsesAPIRequest{
		Model:              req.Model,
		Input:              input,
		Tools:              []responsesToolConfig{tool},
		PreviousResponseID: req.PreviousResponseID,
	}

	jsonData, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logJSONRequest(http.MethodPost, url, httpReq.Header, jsonData)

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

	var apiResp responsesAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("responses API error: %s", apiResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("responses API failed: status %d", resp.StatusCode)
	}

	result := &models.ResponsesResponse{
		ID: apiResp.ID,
	}

	for _, item := range apiResp.Output {
		if item.Type == "image_generation_call" && item.Result != "" {
			imgData, err := base64.StdEncoding.DecodeString(item.Result)
			if err != nil {
				return nil, fmt.Errorf("failed to decode image: %w", err)
			}
			result.Images = append(result.Images, models.GeneratedImage{
				Data:   imgData,
				Base64: item.Result,
				Index:  len(result.Images),
			})
			if item.RevisedPrompt != "" {
				result.RevisedPrompt = item.RevisedPrompt
			}
		}
	}

	if apiResp.Usage != nil {
		result.Cost = p.costCalc.CalculateOCR(req.Model, apiResp.Usage.InputTokens, apiResp.Usage.OutputTokens)
	}

	return result, nil
}

func (p *Provider) logJSONRequest(method, url string, headers http.Header, body []byte) {
	if !p.verbose {
		return
	}
	fmt.Fprintln(os.Stderr, "--- RESPONSES REQUEST ---")
	fmt.Fprintf(os.Stderr, "%s %s\n", method, url)
	for key, values := range headers {
		for _, value := range values {
			if key == "Authorization" {
				value = "[REDACTED]"
			}
			fmt.Fprintf(os.Stderr, "  %s: %s\n", key, value)
		}
	}
	fmt.Fprintf(os.Stderr, "Body: %s\n", string(body))
	fmt.Fprintln(os.Stderr, "------------------------")
}
