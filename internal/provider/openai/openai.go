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
	"time"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultTimeout = 120 * time.Second
)

type apiRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	Background     string `json:"background,omitempty"`
}

type apiResponse struct {
	Created int64       `json:"created"`
	Data    []imageData `json:"data"`
	Error   *apiError   `json:"error,omitempty"`
}

type imageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	registry   *models.ModelRegistry
	verbose    bool
}

func New(cfg *provider.Config, registry *models.ModelRegistry) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, provider.ErrAPIKeyRequired
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	timeout := defaultTimeout
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}

	return &Provider{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		registry: registry,
		verbose:  cfg.Verbose,
	}, nil
}

func (p *Provider) Name() models.ProviderType {
	return models.ProviderOpenAI
}

func (p *Provider) SupportsModel(model string) bool {
	cap, ok := p.registry.Get(model)
	if !ok {
		return false
	}
	return cap.Provider == models.ProviderOpenAI
}

func (p *Provider) ListModels() []string {
	return p.registry.ListByProvider(models.ProviderOpenAI)
}

func (p *Provider) Generate(ctx context.Context, req *models.Request) (*models.Response, error) {
	apiReq := p.buildAPIRequest(req)

	jsonData, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/images/generations"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logRequest(http.MethodPost, url, httpReq.Header, jsonData)

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

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", provider.ErrGenerationFailed, apiResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", provider.ErrGenerationFailed, resp.StatusCode)
	}

	return p.buildResponse(apiResp)
}

func (p *Provider) buildAPIRequest(req *models.Request) *apiRequest {
	apiReq := &apiRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		N:      req.Count,
		Size:   req.Size,
	}

	if req.Quality != "" {
		apiReq.Quality = req.Quality
	}

	switch req.Model {
	case "gpt-image-1":
		if req.Format != "" {
			apiReq.OutputFormat = req.Format.String()
		}
		if req.Transparent {
			apiReq.Background = "transparent"
		}
	case "dall-e-3":
		apiReq.ResponseFormat = "url"
		if req.Style != "" {
			apiReq.Style = req.Style
		}
	case "dall-e-2":
		apiReq.ResponseFormat = "url"
	}

	return apiReq
}

func (p *Provider) buildResponse(apiResp apiResponse) (*models.Response, error) {
	response := &models.Response{
		Images: make([]models.GeneratedImage, 0, len(apiResp.Data)),
	}

	for i, data := range apiResp.Data {
		img := models.GeneratedImage{
			Index: i,
			URL:   data.URL,
		}

		if data.B64JSON != "" {
			img.Base64 = data.B64JSON
			decoded, err := base64.StdEncoding.DecodeString(data.B64JSON)
			if err != nil {
				return nil, fmt.Errorf("failed to decode image %d: %w", i, err)
			}
			img.Data = decoded
		}

		if i == 0 && data.RevisedPrompt != "" {
			response.RevisedPrompt = data.RevisedPrompt
		}

		response.Images = append(response.Images, img)
	}

	return response, nil
}

func (p *Provider) DownloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (p *Provider) logMultipartRequest(method, url string, headers http.Header, req *models.EditRequest) {
	if !p.verbose {
		return
	}

	fmt.Fprintln(os.Stderr, "--- REQUEST ---")
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
	fmt.Fprintln(os.Stderr, "Body (multipart form):")
	fmt.Fprintf(os.Stderr, "  model: %s\n", req.Model)
	fmt.Fprintf(os.Stderr, "  prompt: %s\n", req.Prompt)
	fmt.Fprintf(os.Stderr, "  image: [%d bytes]\n", len(req.Image))
	if len(req.Mask) > 0 {
		fmt.Fprintf(os.Stderr, "  mask: [%d bytes]\n", len(req.Mask))
	}
	if req.Size != "" {
		fmt.Fprintf(os.Stderr, "  size: %s\n", req.Size)
	}
	if req.Count > 0 {
		fmt.Fprintf(os.Stderr, "  n: %d\n", req.Count)
	}
	if req.Format != "" {
		fmt.Fprintf(os.Stderr, "  output_format: %s\n", req.Format)
	}
	fmt.Fprintln(os.Stderr, "---------------")
}

func (p *Provider) logRequest(method, url string, headers http.Header, body []byte) {
	if !p.verbose {
		return
	}

	fmt.Fprintln(os.Stderr, "--- REQUEST ---")
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
	if len(body) > 0 {
		fmt.Fprintln(os.Stderr, "Body:")
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, body, "  ", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "  %s\n", prettyJSON.String())
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", string(body))
		}
	}
	fmt.Fprintln(os.Stderr, "---------------")
}

func (p *Provider) logResponse(statusCode int, headers http.Header, body []byte) {
	if !p.verbose {
		return
	}

	fmt.Fprintln(os.Stderr, "--- RESPONSE ---")
	fmt.Fprintf(os.Stderr, "Status: %d\n", statusCode)
	fmt.Fprintln(os.Stderr, "Headers:")
	for key, values := range headers {
		for _, value := range values {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", key, value)
		}
	}
	if len(body) > 0 {
		fmt.Fprintln(os.Stderr, "Body:")
		// Truncate large base64 data in responses for readability
		truncatedBody := truncateBase64InJSON(body)
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, truncatedBody, "  ", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "  %s\n", prettyJSON.String())
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", string(truncatedBody))
		}
	}
	fmt.Fprintln(os.Stderr, "----------------")
}

func truncateBase64InJSON(body []byte) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	truncateBase64Fields(data)

	result, err := json.Marshal(data)
	if err != nil {
		return body
	}
	return result
}

func truncateBase64Fields(data map[string]interface{}) {
	for key, value := range data {
		switch v := value.(type) {
		case string:
			if key == "b64_json" && len(v) > 100 {
				data[key] = v[:100] + "... [truncated]"
			}
		case map[string]interface{}:
			truncateBase64Fields(v)
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					truncateBase64Fields(m)
				}
			}
		}
	}
}
