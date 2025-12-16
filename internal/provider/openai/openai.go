package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/images/generations", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
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
