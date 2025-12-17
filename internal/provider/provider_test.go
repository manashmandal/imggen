package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/manash/imggen/pkg/models"
)

// mockProvider is a test implementation of Provider.
type mockProvider struct {
	name            models.ProviderType
	supportedModels []string
	generateFunc    func(ctx context.Context, req *models.Request) (*models.Response, error)
}

func (m *mockProvider) Name() models.ProviderType {
	return m.name
}

func (m *mockProvider) Generate(ctx context.Context, req *models.Request) (*models.Response, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return &models.Response{}, nil
}

func (m *mockProvider) Edit(_ context.Context, _ *models.EditRequest) (*models.Response, error) {
	return nil, ErrEditNotSupported
}

func (m *mockProvider) SupportsModel(model string) bool {
	for _, m := range m.supportedModels {
		if m == model {
			return true
		}
	}
	return false
}

func (m *mockProvider) SupportsEdit(_ string) bool {
	return false
}

func (m *mockProvider) ListModels() []string {
	return m.supportedModels
}

func TestNewFactory(t *testing.T) {
	registry := models.NewModelRegistry()
	factory := NewFactory(registry)

	if factory == nil {
		t.Fatal("NewFactory() returned nil")
	}
	if factory.registry != registry {
		t.Error("NewFactory() registry not set correctly")
	}
	if factory.configs == nil {
		t.Error("NewFactory() configs map is nil")
	}
	if factory.providers == nil {
		t.Error("NewFactory() providers map is nil")
	}
}

func TestFactory_Configure(t *testing.T) {
	factory := NewFactory(models.NewModelRegistry())
	cfg := &Config{
		APIKey:     "test-key",
		BaseURL:    "https://test.api.com",
		TimeoutSec: 30,
	}

	factory.Configure(models.ProviderOpenAI, cfg)

	got, ok := factory.GetConfig(models.ProviderOpenAI)
	if !ok {
		t.Fatal("GetConfig() returned false after Configure()")
	}
	if got.APIKey != cfg.APIKey {
		t.Errorf("GetConfig() APIKey = %v, want %v", got.APIKey, cfg.APIKey)
	}
	if got.BaseURL != cfg.BaseURL {
		t.Errorf("GetConfig() BaseURL = %v, want %v", got.BaseURL, cfg.BaseURL)
	}
	if got.TimeoutSec != cfg.TimeoutSec {
		t.Errorf("GetConfig() TimeoutSec = %v, want %v", got.TimeoutSec, cfg.TimeoutSec)
	}
}

func TestFactory_GetConfig_NotFound(t *testing.T) {
	factory := NewFactory(models.NewModelRegistry())

	_, ok := factory.GetConfig(models.ProviderOpenAI)
	if ok {
		t.Error("GetConfig() returned true for unconfigured provider")
	}
}

func TestFactory_Register(t *testing.T) {
	factory := NewFactory(models.NewModelRegistry())
	provider := &mockProvider{name: models.ProviderOpenAI}

	factory.Register(provider)

	got, err := factory.Get(models.ProviderOpenAI)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name() != models.ProviderOpenAI {
		t.Errorf("Get() provider name = %v, want %v", got.Name(), models.ProviderOpenAI)
	}
}

func TestFactory_Get_NotFound(t *testing.T) {
	factory := NewFactory(models.NewModelRegistry())

	_, err := factory.Get(models.ProviderOpenAI)
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("Get() error = %v, want %v", err, ErrProviderNotFound)
	}
}

func TestFactory_GetForModel(t *testing.T) {
	registry := models.NewModelRegistry()
	registry.Register(&models.ModelCapabilities{
		Name:     "test-model",
		Provider: models.ProviderOpenAI,
	})

	factory := NewFactory(registry)
	provider := &mockProvider{name: models.ProviderOpenAI}
	factory.Register(provider)

	got, err := factory.GetForModel("test-model")
	if err != nil {
		t.Fatalf("GetForModel() error = %v", err)
	}
	if got.Name() != models.ProviderOpenAI {
		t.Errorf("GetForModel() provider name = %v, want %v", got.Name(), models.ProviderOpenAI)
	}
}

func TestFactory_GetForModel_UnknownModel(t *testing.T) {
	factory := NewFactory(models.NewModelRegistry())

	_, err := factory.GetForModel("unknown-model")
	if err == nil {
		t.Fatal("GetForModel() error = nil, want error")
	}
	if !errors.Is(err, ErrModelNotSupported) {
		t.Errorf("GetForModel() error = %v, want %v", err, ErrModelNotSupported)
	}
}

func TestFactory_GetForModel_ProviderNotRegistered(t *testing.T) {
	registry := models.NewModelRegistry()
	registry.Register(&models.ModelCapabilities{
		Name:     "test-model",
		Provider: models.ProviderOpenAI,
	})

	factory := NewFactory(registry)
	// Don't register the provider

	_, err := factory.GetForModel("test-model")
	if err == nil {
		t.Fatal("GetForModel() error = nil, want error")
	}
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("GetForModel() error = %v, want %v", err, ErrProviderNotFound)
	}
}

func TestFactory_ListProviders(t *testing.T) {
	factory := NewFactory(models.NewModelRegistry())
	factory.Register(&mockProvider{name: models.ProviderOpenAI})
	factory.Register(&mockProvider{name: models.ProviderStability})

	providers := factory.ListProviders()
	if len(providers) != 2 {
		t.Errorf("ListProviders() returned %d providers, want 2", len(providers))
	}

	found := make(map[models.ProviderType]bool)
	for _, p := range providers {
		found[p] = true
	}

	if !found[models.ProviderOpenAI] {
		t.Error("ListProviders() missing OpenAI")
	}
	if !found[models.ProviderStability] {
		t.Error("ListProviders() missing Stability")
	}
}

func TestErrors(t *testing.T) {
	// Ensure error variables are properly defined
	if ErrProviderNotFound == nil {
		t.Error("ErrProviderNotFound is nil")
	}
	if ErrModelNotSupported == nil {
		t.Error("ErrModelNotSupported is nil")
	}
	if ErrAPIKeyRequired == nil {
		t.Error("ErrAPIKeyRequired is nil")
	}
	if ErrGenerationFailed == nil {
		t.Error("ErrGenerationFailed is nil")
	}
}
