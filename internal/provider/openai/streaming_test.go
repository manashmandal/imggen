package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestProcessStream_PartialThenCompleted(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"image_generation.partial_image","b64_json":"AAEC","partial_image_index":0,"size":"1024x1024","quality":"medium"}`,
		``,
		`data: {"type":"image_generation.partial_image","b64_json":"AwQF","partial_image_index":1,"size":"1024x1024","quality":"medium"}`,
		``,
		`data: {"type":"image_generation.completed","b64_json":"BgcICQ==","size":"1024x1024","quality":"medium","usage":{"input_tokens":120,"output_tokens":4310,"total_tokens":4430,"input_tokens_details":{"text_tokens":100,"image_tokens":20}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var events []*models.StreamEvent
	handler := provider.StreamHandler(func(ev *models.StreamEvent) {
		events = append(events, ev)
	})

	resp, err := processStream(strings.NewReader(raw), handler)
	if err != nil {
		t.Fatalf("processStream() error: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Type != models.StreamEventPartial || events[0].Index != 0 {
		t.Errorf("event[0]: type=%s index=%d, want partial 0", events[0].Type, events[0].Index)
	}
	if events[1].Type != models.StreamEventPartial || events[1].Index != 1 {
		t.Errorf("event[1]: type=%s index=%d, want partial 1", events[1].Type, events[1].Index)
	}
	if events[2].Type != models.StreamEventCompleted {
		t.Errorf("event[2]: type=%s, want completed", events[2].Type)
	}
	if events[2].Usage == nil || events[2].Usage.InputTokens != 120 || events[2].Usage.TotalTokens != 4430 {
		t.Errorf("event[2] usage = %+v, want input=120 total=4430", events[2].Usage)
	}

	if len(resp.Images) != 1 {
		t.Fatalf("got %d images, want 1", len(resp.Images))
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 4430 {
		t.Errorf("resp.Usage = %+v, want total=4430", resp.Usage)
	}
	if resp.Usage.TextInputTokens != 100 || resp.Usage.ImageInputTokens != 20 {
		t.Errorf("input details: text=%d image=%d, want text=100 image=20",
			resp.Usage.TextInputTokens, resp.Usage.ImageInputTokens)
	}
	decoded, _ := base64.StdEncoding.DecodeString("BgcICQ==")
	if !bytes.Equal(resp.Images[0].Data, decoded) {
		t.Errorf("final image bytes mismatch: got %v want %v", resp.Images[0].Data, decoded)
	}
}

func TestProcessStream_EditEvents(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"image_edit.partial_image","b64_json":"AAEC","partial_image_index":0}`,
		``,
		`data: {"type":"image_edit.completed","b64_json":"BgcICQ=="}`,
		``,
	}, "\n")

	resp, err := processStream(strings.NewReader(raw), nil)
	if err != nil {
		t.Fatalf("processStream() error: %v", err)
	}
	if len(resp.Images) != 1 {
		t.Fatalf("got %d images, want 1", len(resp.Images))
	}
}

func TestProcessStream_NoCompletedEvent(t *testing.T) {
	raw := "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"AAEC\",\"partial_image_index\":0}\n\n"
	if _, err := processStream(strings.NewReader(raw), nil); err == nil {
		t.Fatal("expected error for missing completed event, got nil")
	}
}

func TestProcessStream_InvalidJSON(t *testing.T) {
	raw := "data: {bad json\n\n"
	if _, err := processStream(strings.NewReader(raw), nil); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestProcessStream_NilHandlerStillWorks(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"image_generation.partial_image","b64_json":"AAEC","partial_image_index":0}`,
		``,
		`data: {"type":"image_generation.completed","b64_json":"BgcICQ=="}`,
		``,
	}, "\n")
	resp, err := processStream(strings.NewReader(raw), nil)
	if err != nil {
		t.Fatalf("nil handler should not error: %v", err)
	}
	if len(resp.Images) != 1 {
		t.Errorf("got %d images, want 1", len(resp.Images))
	}
}

func TestGenerateStream_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept header = %q, want text/event-stream", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("body decode: %v", err)
		}
		if body["stream"] != true {
			t.Errorf("stream not true in request: %+v", body)
		}
		if body["partial_images"] != float64(2) {
			t.Errorf("partial_images = %v, want 2", body["partial_images"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`data: {"type":"image_generation.partial_image","b64_json":"AAEC","partial_image_index":0}`,
			``,
			`data: {"type":"image_generation.partial_image","b64_json":"AwQF","partial_image_index":1}`,
			``,
			`data: {"type":"image_generation.completed","b64_json":"BgcICQ==","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`,
			``,
		}, "\n")
		_, _ = w.Write([]byte(sse))
	}))
	defer server.Close()

	p, err := New(&provider.Config{APIKey: "test", BaseURL: server.URL}, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := models.NewRequest("test")
	req.Model = "gpt-image-2"
	req.PartialImages = 2

	var partials []int
	handler := provider.StreamHandler(func(ev *models.StreamEvent) {
		if ev.Type == models.StreamEventPartial {
			partials = append(partials, ev.Index)
		}
	})

	resp, err := p.GenerateStream(context.Background(), req, handler)
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	if len(partials) != 2 || partials[0] != 0 || partials[1] != 1 {
		t.Errorf("partials = %v, want [0 1]", partials)
	}
	if len(resp.Images) != 1 {
		t.Errorf("got %d images, want 1", len(resp.Images))
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Errorf("usage = %+v, want total=3", resp.Usage)
	}
	// Token-based pricing: 1 input * $5/1M + 2 output * $30/1M = 0.000005 + 0.00006 = 0.000065.
	if resp.Cost == nil {
		t.Fatal("expected Cost to be populated from token usage")
	}
	want := 0.000065
	if diff := resp.Cost.Total - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("token-based cost = %.7f, want %.7f", resp.Cost.Total, want)
	}
}

func TestGenerateStream_RejectsNonGPT2(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	req := models.NewRequest("test")
	req.Model = "gpt-image-1.5"
	_, err := p.GenerateStream(context.Background(), req, nil)
	if !errors.Is(err, ErrStreamingNotSupported) {
		t.Errorf("got %v, want ErrStreamingNotSupported", err)
	}
}

func TestGenerateStream_RejectsBadPartialImagesValue(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"negative", -1},
		{"too-large", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
			req := models.NewRequest("test")
			req.Model = "gpt-image-2"
			req.PartialImages = tt.value
			if _, err := p.GenerateStream(context.Background(), req, nil); err == nil {
				t.Errorf("partial_images=%d should error", tt.value)
			}
		})
	}
}

func TestEditStream_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Content-Type = %q, want multipart/form-data", ct)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}

		// Parse multipart to confirm stream=true and partial_images=N are set.
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		if got := r.FormValue("stream"); got != "true" {
			t.Errorf("stream = %q, want true", got)
		}
		if got := r.FormValue("partial_images"); got != "1" {
			t.Errorf("partial_images = %q, want 1", got)
		}
		if got := r.FormValue("model"); got != "gpt-image-2" {
			t.Errorf("model = %q, want gpt-image-2", got)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		sse := strings.Join([]string{
			`data: {"type":"image_edit.partial_image","b64_json":"AAEC","partial_image_index":0}`,
			``,
			`data: {"type":"image_edit.completed","b64_json":"AwQF","usage":{"input_tokens":5,"output_tokens":1000,"total_tokens":1005}}`,
			``,
		}, "\n")
		_, _ = w.Write([]byte(sse))
	}))
	defer server.Close()

	p, err := New(&provider.Config{APIKey: "test", BaseURL: server.URL}, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := models.NewEditRequest([]byte("fake-image-data"), "test edit")
	req.Model = "gpt-image-2"
	req.PartialImages = 1

	var partials []int
	handler := provider.StreamHandler(func(ev *models.StreamEvent) {
		if ev.Type == models.StreamEventPartial {
			partials = append(partials, ev.Index)
		}
	})

	resp, err := p.EditStream(context.Background(), req, handler)
	if err != nil {
		t.Fatalf("EditStream: %v", err)
	}
	if len(partials) != 1 || partials[0] != 0 {
		t.Errorf("partials = %v, want [0]", partials)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 1005 {
		t.Errorf("usage = %+v, want total=1005", resp.Usage)
	}
	// Token-based: 5 * 5/1M + 1000 * 30/1M = 0.000025 + 0.03 = 0.030025
	if resp.Cost == nil {
		t.Fatal("Cost should be populated from token usage")
	}
	want := 0.030025
	if diff := resp.Cost.Total - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("token-based edit cost = %.7f, want %.7f", resp.Cost.Total, want)
	}
}

func TestEditStream_RejectsBadPartialImagesValue(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	req := models.NewEditRequest([]byte("fake"), "test")
	req.Model = "gpt-image-2"
	req.PartialImages = 5
	if _, err := p.EditStream(context.Background(), req, nil); err == nil {
		t.Error("partial_images=5 should error")
	}
}

func TestStreamCost_PrefersTokenUsage(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	resp := &models.Response{
		Images: []models.GeneratedImage{{Index: 0}},
		Usage:  &models.TokenUsage{InputTokens: 10, OutputTokens: 1000, TotalTokens: 1010},
	}
	cost := p.streamCost("gpt-image-2", "1024x1024", "low", resp)
	// 10 * 5/1M + 1000 * 30/1M = 0.00005 + 0.03 = 0.03005
	want := 0.03005
	if diff := cost.Total - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("token-based cost = %.6f, want %.6f", cost.Total, want)
	}
}

func TestStreamCost_FallsBackToFlatRate(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	resp := &models.Response{
		Images: []models.GeneratedImage{{Index: 0}},
		Usage:  nil,
	}
	cost := p.streamCost("gpt-image-2", "1024x1024", "low", resp)
	if cost.Total != 0.006 {
		t.Errorf("flat-rate cost = %.4f, want 0.006", cost.Total)
	}
}

func TestEditStream_RejectsNonGPT2(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	req := models.NewEditRequest([]byte("fake"), "test")
	req.Model = "gpt-image-1.5"
	_, err := p.EditStream(context.Background(), req, nil)
	if !errors.Is(err, ErrStreamingNotSupported) {
		t.Errorf("got %v, want ErrStreamingNotSupported", err)
	}
}
