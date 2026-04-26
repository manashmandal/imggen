package openai

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

// streamingModelOnly identifies models that accept stream=true on the image
// endpoints. Older GPT image models reject the parameter.
const streamingModelOnly = "gpt-image-2"

// ErrStreamingNotSupported is returned when streaming is requested for a
// model that does not support it.
var ErrStreamingNotSupported = errors.New("streaming not supported by model")

type streamUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type sseEvent struct {
	Type              string       `json:"type"`
	B64JSON           string       `json:"b64_json"`
	PartialImageIndex int          `json:"partial_image_index"`
	Size              string       `json:"size"`
	Quality           string       `json:"quality"`
	Usage             *streamUsage `json:"usage,omitempty"`
}

// processStream reads SSE events from r, dispatching each to onEvent, and
// returns a Response built from the completed event. The handler may be nil.
// Returns an error if the stream ends without a completed event.
func processStream(r io.Reader, onEvent provider.StreamHandler) (*models.Response, error) {
	scanner := bufio.NewScanner(r)
	// Partial/final image base64 payloads can be hundreds of KB.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var final *sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var ev sseEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			return nil, fmt.Errorf("parse stream event: %w", err)
		}

		switch {
		case strings.HasSuffix(ev.Type, ".partial_image"):
			if err := dispatchPartial(&ev, onEvent); err != nil {
				return nil, err
			}
		case strings.HasSuffix(ev.Type, ".completed"):
			completed := ev
			final = &completed
			if err := dispatchCompleted(&ev, onEvent); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}
	if final == nil {
		return nil, errors.New("stream ended without completed event")
	}

	data, err := base64.StdEncoding.DecodeString(final.B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode final image: %w", err)
	}
	resp := &models.Response{
		Images: []models.GeneratedImage{{Data: data, Base64: final.B64JSON, Index: 0}},
	}
	if final.Usage != nil {
		resp.Usage = &models.TokenUsage{
			InputTokens:  final.Usage.InputTokens,
			OutputTokens: final.Usage.OutputTokens,
			TotalTokens:  final.Usage.TotalTokens,
		}
	}
	return resp, nil
}

func dispatchPartial(ev *sseEvent, onEvent provider.StreamHandler) error {
	if onEvent == nil {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(ev.B64JSON)
	if err != nil {
		return fmt.Errorf("decode partial %d: %w", ev.PartialImageIndex, err)
	}
	onEvent(&models.StreamEvent{
		Type:    models.StreamEventPartial,
		Index:   ev.PartialImageIndex,
		Data:    data,
		Base64:  ev.B64JSON,
		Size:    ev.Size,
		Quality: ev.Quality,
	})
	return nil
}

func dispatchCompleted(ev *sseEvent, onEvent provider.StreamHandler) error {
	if onEvent == nil {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(ev.B64JSON)
	if err != nil {
		return fmt.Errorf("decode final image: %w", err)
	}
	out := &models.StreamEvent{
		Type:    models.StreamEventCompleted,
		Index:   -1,
		Data:    data,
		Base64:  ev.B64JSON,
		Size:    ev.Size,
		Quality: ev.Quality,
	}
	if ev.Usage != nil {
		out.Usage = &models.TokenUsage{
			InputTokens:  ev.Usage.InputTokens,
			OutputTokens: ev.Usage.OutputTokens,
			TotalTokens:  ev.Usage.TotalTokens,
		}
	}
	onEvent(out)
	return nil
}

// streamHTTP runs an HTTP request expected to return text/event-stream and
// dispatches events. The caller owns the request body.
func (p *Provider) streamHTTP(ctx context.Context, req *http.Request, onEvent provider.StreamHandler) (*models.Response, error) {
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Use a streaming-safe client: no overall timeout, since SSE bodies last
	// for the full generation. Cancellation is via ctx.
	client := &http.Client{}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("send streaming request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		p.logResponse(resp.StatusCode, resp.Header, body)
		return nil, fmt.Errorf("%w: status %d: %s", provider.ErrGenerationFailed, resp.StatusCode, body)
	}

	return processStream(resp.Body, onEvent)
}
