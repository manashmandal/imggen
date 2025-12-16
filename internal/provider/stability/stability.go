package stability

import (
	"context"
	"errors"

	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

var ErrNotImplemented = errors.New("stability AI provider not yet implemented")

type Provider struct {
	apiKey   string
	baseURL  string
	registry *models.ModelRegistry
}

func New(cfg *provider.Config, registry *models.ModelRegistry) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, provider.ErrAPIKeyRequired
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.stability.ai/v1"
	}

	return &Provider{
		apiKey:   cfg.APIKey,
		baseURL:  baseURL,
		registry: registry,
	}, nil
}

func (p *Provider) Name() models.ProviderType {
	return models.ProviderStability
}

func (p *Provider) SupportsModel(model string) bool {
	cap, ok := p.registry.Get(model)
	if !ok {
		return false
	}
	return cap.Provider == models.ProviderStability
}

func (p *Provider) ListModels() []string {
	return p.registry.ListByProvider(models.ProviderStability)
}

func (p *Provider) Generate(_ context.Context, _ *models.Request) (*models.Response, error) {
	return nil, ErrNotImplemented
}
