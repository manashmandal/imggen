package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/manash/imggen/pkg/models"
)

var (
	ErrProviderNotFound  = errors.New("provider not found")
	ErrModelNotSupported = errors.New("model not supported by provider")
	ErrAPIKeyRequired    = errors.New("API key is required")
	ErrGenerationFailed  = errors.New("image generation failed")
	ErrEditFailed        = errors.New("image edit failed")
	ErrEditNotSupported  = errors.New("image editing not supported by model")
)

type Provider interface {
	Name() models.ProviderType
	Generate(ctx context.Context, req *models.Request) (*models.Response, error)
	Edit(ctx context.Context, req *models.EditRequest) (*models.Response, error)
	SupportsModel(model string) bool
	SupportsEdit(model string) bool
	ListModels() []string
}

type Config struct {
	APIKey     string
	BaseURL    string
	TimeoutSec int
}

type Factory struct {
	registry  *models.ModelRegistry
	configs   map[models.ProviderType]*Config
	providers map[models.ProviderType]Provider
}

func NewFactory(registry *models.ModelRegistry) *Factory {
	return &Factory{
		registry:  registry,
		configs:   make(map[models.ProviderType]*Config),
		providers: make(map[models.ProviderType]Provider),
	}
}

func (f *Factory) Configure(providerType models.ProviderType, cfg *Config) {
	f.configs[providerType] = cfg
}

func (f *Factory) Register(provider Provider) {
	f.providers[provider.Name()] = provider
}

func (f *Factory) Get(providerType models.ProviderType) (Provider, error) {
	provider, ok := f.providers[providerType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, providerType)
	}
	return provider, nil
}

func (f *Factory) GetForModel(model string) (Provider, error) {
	cap, ok := f.registry.Get(model)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrModelNotSupported, model)
	}

	provider, ok := f.providers[cap.Provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s (required by model %s)", ErrProviderNotFound, cap.Provider, model)
	}

	return provider, nil
}

func (f *Factory) GetConfig(providerType models.ProviderType) (*Config, bool) {
	cfg, ok := f.configs[providerType]
	return cfg, ok
}

func (f *Factory) ListProviders() []models.ProviderType {
	types := make([]models.ProviderType, 0, len(f.providers))
	for t := range f.providers {
		types = append(types, t)
	}
	return types
}
