package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func (p *Provider) SupportsEdit(model string) bool {
	cap, ok := p.registry.Get(model)
	if !ok {
		return false
	}
	return cap.SupportsEdit && cap.Provider == models.ProviderOpenAI
}

func (p *Provider) Edit(ctx context.Context, req *models.EditRequest) (*models.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if !p.SupportsEdit(req.Model) {
		return nil, fmt.Errorf("%w: %s", provider.ErrEditNotSupported, req.Model)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	imagePart, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return nil, fmt.Errorf("failed to create image part: %w", err)
	}
	if _, err := imagePart.Write(req.Image); err != nil {
		return nil, fmt.Errorf("failed to write image: %w", err)
	}

	if len(req.Mask) > 0 {
		maskPart, err := writer.CreateFormFile("mask", "mask.png")
		if err != nil {
			return nil, fmt.Errorf("failed to create mask part: %w", err)
		}
		if _, err := maskPart.Write(req.Mask); err != nil {
			return nil, fmt.Errorf("failed to write mask: %w", err)
		}
	}

	if err := writer.WriteField("prompt", req.Prompt); err != nil {
		return nil, fmt.Errorf("failed to write prompt: %w", err)
	}

	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, fmt.Errorf("failed to write model: %w", err)
	}

	if req.Size != "" {
		if err := writer.WriteField("size", req.Size); err != nil {
			return nil, fmt.Errorf("failed to write size: %w", err)
		}
	}

	if req.Count > 0 {
		if err := writer.WriteField("n", fmt.Sprintf("%d", req.Count)); err != nil {
			return nil, fmt.Errorf("failed to write count: %w", err)
		}
	}

	if req.Model == "gpt-image-1" && req.Format != "" {
		if err := writer.WriteField("output_format", req.Format.String()); err != nil {
			return nil, fmt.Errorf("failed to write output_format: %w", err)
		}
	} else if req.Model == "dall-e-2" {
		if err := writer.WriteField("response_format", "url"); err != nil {
			return nil, fmt.Errorf("failed to write response_format: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/images/edits", body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", provider.ErrEditFailed, apiResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", provider.ErrEditFailed, resp.StatusCode)
	}

	return p.buildResponse(apiResp)
}
