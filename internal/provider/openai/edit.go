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

	body, contentType, err := buildEditMultipart(req, false)
	if err != nil {
		return nil, err
	}

	url := p.baseURL + "/images/edits"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", contentType)
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

	// Edit operations use medium quality for GPT image models, no quality for dall-e-2
	quality := req.Quality
	if isGPTImageModel(req.Model) && quality == "" {
		quality = "auto"
	}
	response.Cost = p.costCalc.Calculate(models.ProviderOpenAI, req.Model, req.Size, quality, len(response.Images))
	return response, nil
}

// EditStream invokes the image edit endpoint with stream=true and dispatches
// each SSE event to onEvent. Only gpt-image-2 supports streaming.
func (p *Provider) EditStream(ctx context.Context, req *models.EditRequest, onEvent provider.StreamHandler) (*models.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.Model != streamingModelOnly {
		return nil, fmt.Errorf("%w: %s", ErrStreamingNotSupported, req.Model)
	}
	if req.PartialImages < 0 || req.PartialImages > 3 {
		return nil, fmt.Errorf("partial_images must be 0-3, got %d", req.PartialImages)
	}
	if !p.SupportsEdit(req.Model) {
		return nil, fmt.Errorf("%w: %s", provider.ErrEditNotSupported, req.Model)
	}

	body, contentType, err := buildEditMultipart(req, true)
	if err != nil {
		return nil, err
	}

	url := p.baseURL + "/images/edits"
	httpReq, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)

	p.logMultipartRequest(http.MethodPost, url, httpReq.Header, req)

	response, err := p.streamHTTP(ctx, httpReq, onEvent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", provider.ErrEditFailed, err)
	}

	quality := req.Quality
	if isGPTImageModel(req.Model) && quality == "" {
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

// buildEditMultipart serializes an EditRequest as multipart/form-data. The
// stream flag adds stream=true (and partial_images, when non-zero) for the
// streaming endpoint variant.
func buildEditMultipart(req *models.EditRequest, stream bool) (*bytes.Buffer, string, error) {
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
		ct := mimeTypes[i]
		if ct == "" {
			ct = detectMimeType(img)
		}
		part, err := createFormFileWithContentType(writer, fieldName, fmt.Sprintf("image-%d.png", i+1), ct)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create image part: %w", err)
		}
		if _, err := part.Write(img); err != nil {
			return nil, "", fmt.Errorf("failed to write image: %w", err)
		}
	}

	if len(req.Mask) > 0 {
		maskPart, err := createFormFileWithContentType(writer, "mask", "mask.png", "image/png")
		if err != nil {
			return nil, "", fmt.Errorf("failed to create mask part: %w", err)
		}
		if _, err := maskPart.Write(req.Mask); err != nil {
			return nil, "", fmt.Errorf("failed to write mask: %w", err)
		}
	}

	fields := []struct {
		name, value string
		write       bool
	}{
		{"prompt", req.Prompt, true},
		{"model", req.Model, true},
		{"size", req.Size, req.Size != ""},
		{"n", fmt.Sprintf("%d", req.Count), req.Count > 0},
		{"quality", req.Quality, req.Quality != ""},
		{"background", req.Background, req.Background != ""},
	}
	for _, f := range fields {
		if !f.write {
			continue
		}
		if err := writer.WriteField(f.name, f.value); err != nil {
			return nil, "", fmt.Errorf("failed to write %s: %w", f.name, err)
		}
	}

	if req.InputFidelity != "" && supportsInputFidelity(req.Model) {
		if err := writer.WriteField("input_fidelity", req.InputFidelity); err != nil {
			return nil, "", fmt.Errorf("failed to write input_fidelity: %w", err)
		}
	}

	switch {
	case isGPTImageModel(req.Model):
		if req.Format != "" {
			if err := writer.WriteField("output_format", req.Format.String()); err != nil {
				return nil, "", fmt.Errorf("failed to write output_format: %w", err)
			}
		}
		if req.OutputCompression != nil {
			if err := writer.WriteField("output_compression", fmt.Sprintf("%d", *req.OutputCompression)); err != nil {
				return nil, "", fmt.Errorf("failed to write output_compression: %w", err)
			}
		}
		if req.Moderation != "" {
			if err := writer.WriteField("moderation", req.Moderation); err != nil {
				return nil, "", fmt.Errorf("failed to write moderation: %w", err)
			}
		}
	case req.Model == "dall-e-2":
		if err := writer.WriteField("response_format", "url"); err != nil {
			return nil, "", fmt.Errorf("failed to write response_format: %w", err)
		}
	}

	if stream {
		if err := writer.WriteField("stream", "true"); err != nil {
			return nil, "", fmt.Errorf("failed to write stream: %w", err)
		}
		if req.PartialImages > 0 {
			if err := writer.WriteField("partial_images", fmt.Sprintf("%d", req.PartialImages)); err != nil {
				return nil, "", fmt.Errorf("failed to write partial_images: %w", err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}
	return body, writer.FormDataContentType(), nil
}
