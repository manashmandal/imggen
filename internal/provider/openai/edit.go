package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"

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

	images := req.Images
	mimeTypes := req.ImageMimeTypes
	if len(images) == 0 && len(req.Image) > 0 {
		images = [][]byte{req.Image}
	}
	if len(mimeTypes) < len(images) {
		mimeTypes = append(mimeTypes, make([]string, len(images)-len(mimeTypes))...)
	}

	for i, img := range images {
		fieldName := "image"
		if len(images) > 1 {
			fieldName = "image[]"
		}
		contentType := mimeTypes[i]
		if contentType == "" {
			contentType = detectMimeType(img)
		}
		imagePart, err := createFormFileWithContentType(writer, fieldName, fmt.Sprintf("image-%d.png", i+1), contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to create image part: %w", err)
		}
		if _, err := imagePart.Write(img); err != nil {
			return nil, fmt.Errorf("failed to write image: %w", err)
		}
	}

	if len(req.Mask) > 0 {
		maskPart, err := createFormFileWithContentType(writer, "mask", "mask.png", "image/png")
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

	if req.Quality != "" {
		if err := writer.WriteField("quality", req.Quality); err != nil {
			return nil, fmt.Errorf("failed to write quality: %w", err)
		}
	}

	if req.Background != "" {
		if err := writer.WriteField("background", req.Background); err != nil {
			return nil, fmt.Errorf("failed to write background: %w", err)
		}
	}

	if req.InputFidelity != "" {
		if err := writer.WriteField("input_fidelity", req.InputFidelity); err != nil {
			return nil, fmt.Errorf("failed to write input_fidelity: %w", err)
		}
	}

	if (req.Model == "gpt-image-1.5" || req.Model == "gpt-image-1") && req.Format != "" {
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

	url := p.baseURL + "/images/edits"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logMultipartRequest(http.MethodPost, url, httpReq.Header, req)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	p.logResponse(resp.StatusCode, resp.Header, bodyBytes)

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

	response, err := p.buildResponse(apiResp)
	if err != nil {
		return nil, err
	}

	// Edit operations use medium quality for gpt-image-1, no quality for dall-e-2
	quality := req.Quality
	if (req.Model == "gpt-image-1.5" || req.Model == "gpt-image-1") && quality == "" {
		quality = "auto"
	}
	response.Cost = p.costCalc.Calculate(models.ProviderOpenAI, req.Model, req.Size, quality, len(response.Images))
	return response, nil
}

func createFormFileWithContentType(w *multipart.Writer, fieldname, filename, contentType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldname, filename))
	h.Set("Content-Type", contentType)
	return w.CreatePart(h)
}
