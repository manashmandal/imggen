package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

var (
	defaultPollInterval = 2 * time.Second
	maxPollAttempts     = 300 // 10 minutes max at 2s intervals
)

type videoJobResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	CreatedAt int64           `json:"created_at"`
	Status    string          `json:"status"`
	Model     string          `json:"model"`
	Progress  json.RawMessage `json:"progress,omitempty"` // Can be int or string
	Seconds   json.RawMessage `json:"seconds,omitempty"`  // Can be int or string
	Size      string          `json:"size,omitempty"`
	Error     *apiError       `json:"error,omitempty"`
}

// GenerateVideo generates a video using OpenAI's Sora API
func (p *Provider) GenerateVideo(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	jobResp, err := p.createVideoJob(ctx, req)
	if err != nil {
		return nil, err
	}

	completedJob, err := p.pollVideoStatus(ctx, jobResp.ID)
	if err != nil {
		return nil, err
	}

	videoData, err := p.downloadVideo(ctx, completedJob.ID)
	if err != nil {
		return nil, err
	}

	response := &models.VideoResponse{
		Videos: []models.GeneratedVideo{
			{
				Data:     videoData,
				Filename: completedJob.ID + ".mp4",
			},
		},
		Cost: p.costCalc.CalculateVideo(models.ProviderOpenAI, req.Model, req.Duration),
	}

	return response, nil
}

func (p *Provider) createVideoJob(ctx context.Context, req *models.VideoRequest) (*videoJobResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("prompt", req.Prompt); err != nil {
		return nil, fmt.Errorf("failed to write prompt field: %w", err)
	}
	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}
	if req.Duration > 0 {
		if err := writer.WriteField("seconds", strconv.Itoa(req.Duration)); err != nil {
			return nil, fmt.Errorf("failed to write seconds field: %w", err)
		}
	}
	if req.Size != "" {
		if err := writer.WriteField("size", req.Size); err != nil {
			return nil, fmt.Errorf("failed to write size field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to create form: %w", err)
	}

	url := p.baseURL + "/videos"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logRequest(http.MethodPost, url, httpReq.Header, body.Bytes())

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	p.logResponse(resp.StatusCode, resp.Header, respBody)

	var jobResp videoJobResponse
	if err := json.Unmarshal(respBody, &jobResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if jobResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", provider.ErrVideoGenerationFailed, jobResp.Error.Message)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%w: status %d", provider.ErrVideoGenerationFailed, resp.StatusCode)
	}

	return &jobResp, nil
}

func (p *Provider) pollVideoStatus(ctx context.Context, videoID string) (*videoJobResponse, error) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for attempt := 0; attempt < maxPollAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			job, err := p.getVideoStatus(ctx, videoID)
			if err != nil {
				return nil, err
			}

			switch job.Status {
			case "completed":
				return job, nil
			case "failed":
				errMsg := "video generation failed"
				if job.Error != nil {
					errMsg = job.Error.Message
				}
				return nil, fmt.Errorf("%w: %s", provider.ErrVideoGenerationFailed, errMsg)
			case "queued", "in_progress":
				continue
			default:
				return nil, fmt.Errorf("unknown video status: %s", job.Status)
			}
		}
	}

	return nil, fmt.Errorf("%w: exceeded maximum poll attempts", provider.ErrVideoNotReady)
}

func (p *Provider) getVideoStatus(ctx context.Context, videoID string) (*videoJobResponse, error) {
	url := p.baseURL + "/videos/" + videoID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

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

	var jobResp videoJobResponse
	if err := json.Unmarshal(body, &jobResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &jobResp, nil
}

func (p *Provider) downloadVideo(ctx context.Context, videoID string) ([]byte, error) {
	url := p.baseURL + "/videos/" + videoID + "/content"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", provider.ErrVideoDownloadFailed, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// SupportsVideoModel checks if the provider supports the given video model
func (p *Provider) SupportsVideoModel(model string) bool {
	cap, ok := p.registry.GetVideo(model)
	if !ok {
		return false
	}
	return cap.Provider == models.ProviderOpenAI
}

// ListVideoModels returns all video models supported by this provider
func (p *Provider) ListVideoModels() []string {
	var videoModels []string
	for _, name := range p.registry.ListVideoModels() {
		cap, ok := p.registry.GetVideo(name)
		if ok && cap.Provider == models.ProviderOpenAI {
			videoModels = append(videoModels, name)
		}
	}
	return videoModels
}
