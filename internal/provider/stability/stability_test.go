package stability

import (
	"context"
	"errors"
	"testing"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestNew(t *testing.T) {
	registry := models.DefaultRegistry()

	tests := []struct {
		name    string
		cfg     *provider.Config
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     &provider.Config{APIKey: "test-key"},
			wantErr: nil,
		},
		{
			name:    "empty API key",
			cfg:     &provider.Config{APIKey: ""},
			wantErr: provider.ErrAPIKeyRequired,
		},
		{
			name:    "custom base URL",
			cfg:     &provider.Config{APIKey: "test-key", BaseURL: "https://custom.stability.ai"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(tt.cfg, registry)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("New() error = nil, want error")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("New() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
			if p == nil {
				t.Fatal("New() returned nil provider")
			}
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	if p.Name() != models.ProviderStability {
		t.Errorf("Name() = %v, want %v", p.Name(), models.ProviderStability)
	}
}

func TestProvider_SupportsModel(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	tests := []struct {
		model string
		want  bool
	}{
		{"stable-diffusion-xl", true},
		{"stable-diffusion-3", true},
		{"gpt-image-1", false},
		{"dall-e-3", false},
		{"unknown-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := p.SupportsModel(tt.model); got != tt.want {
				t.Errorf("SupportsModel(%s) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestProvider_ListModels(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())
	modelsList := p.ListModels()

	expected := map[string]bool{
		"stable-diffusion-xl": true,
		"stable-diffusion-3":  true,
	}

	if len(modelsList) != len(expected) {
		t.Errorf("ListModels() returned %d models, want %d", len(modelsList), len(expected))
	}

	for _, model := range modelsList {
		if !expected[model] {
			t.Errorf("ListModels() unexpected model: %s", model)
		}
	}
}

func TestProvider_Generate_NotImplemented(t *testing.T) {
	p, _ := New(&provider.Config{APIKey: "test"}, models.DefaultRegistry())

	req := &models.Request{
		Model:  "stable-diffusion-xl",
		Prompt: "test prompt",
		Count:  1,
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("Generate() error = nil, want ErrNotImplemented")
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Generate() error = %v, want %v", err, ErrNotImplemented)
	}
}

func TestNew_DefaultBaseURL(t *testing.T) {
	cfg := &provider.Config{APIKey: "test-key"}
	p, err := New(cfg, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if p.baseURL != "https://api.stability.ai/v1" {
		t.Errorf("New() baseURL = %v, want https://api.stability.ai/v1", p.baseURL)
	}
}

func TestNew_CustomBaseURL(t *testing.T) {
	cfg := &provider.Config{
		APIKey:  "test-key",
		BaseURL: "https://custom.api.com",
	}
	p, err := New(cfg, models.DefaultRegistry())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if p.baseURL != "https://custom.api.com" {
		t.Errorf("New() baseURL = %v, want https://custom.api.com", p.baseURL)
	}
}

func TestErrNotImplemented(t *testing.T) {
	if ErrNotImplemented == nil {
		t.Error("ErrNotImplemented is nil")
	}
	if ErrNotImplemented.Error() != "stability AI provider not yet implemented" {
		t.Errorf("ErrNotImplemented message = %v", ErrNotImplemented.Error())
	}
}
