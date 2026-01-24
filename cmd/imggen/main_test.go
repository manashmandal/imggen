package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/manash/imggen/internal/display"
	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/internal/session"
	"github.com/manash/imggen/pkg/models"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	generateFunc func(ctx context.Context, req *models.Request) (*models.Response, error)
}

func (m *mockProvider) Name() models.ProviderType {
	return models.ProviderOpenAI
}

func (m *mockProvider) Generate(ctx context.Context, req *models.Request) (*models.Response, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return &models.Response{
		Images: []models.GeneratedImage{
			{Data: []byte("test image data"), Index: 0},
		},
	}, nil
}

func (m *mockProvider) Edit(_ context.Context, _ *models.EditRequest) (*models.Response, error) {
	return nil, provider.ErrEditNotSupported
}

func (m *mockProvider) SupportsModel(_ string) bool {
	return true
}

func (m *mockProvider) SupportsEdit(_ string) bool {
	return false
}

func (m *mockProvider) ListModels() []string {
	return []string{"gpt-image-1", "dall-e-3", "dall-e-2"}
}

// resetFlags resets all global flags to their default values.
func resetFlags() {
	flagModel = "gpt-image-1"
	flagSize = ""
	flagQuality = ""
	flagCount = 1
	flagOutput = ""
	flagFormat = "png"
	flagStyle = ""
	flagTransparent = false
	flagAPIKey = ""
	flagShow = false
	flagInteractive = false
	flagDBBackup = false
	flagRegisterDryRun = false
	flagRegisterForce = false
	// Video flags
	flagVideoModel = "sora-2"
	flagVideoDuration = 0
	flagVideoSize = ""
	flagVideoOutput = ""
}

// newTestApp creates an App configured for testing.
func newTestApp(out *bytes.Buffer) *App {
	return &App{
		Out:      out,
		Err:      out,
		Registry: models.DefaultRegistry(),
		GetEnv: func(key string) string {
			return ""
		},
		NewProvider: func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
			return &mockProvider{}, nil
		},
		NewSaver:     image.NewSaver,
		NewDisplayer: display.New,
	}
}

func TestDefaultApp(t *testing.T) {
	app := DefaultApp()

	if app.Out == nil {
		t.Error("DefaultApp() Out is nil")
	}
	if app.Err == nil {
		t.Error("DefaultApp() Err is nil")
	}
	if app.Registry == nil {
		t.Error("DefaultApp() Registry is nil")
	}
	if app.GetEnv == nil {
		t.Error("DefaultApp() GetEnv is nil")
	}
	if app.NewProvider == nil {
		t.Error("DefaultApp() NewProvider is nil")
	}
	if app.NewSaver == nil {
		t.Error("DefaultApp() NewSaver is nil")
	}
	if app.NewDisplayer == nil {
		t.Error("DefaultApp() NewDisplayer is nil")
	}

	// Test GetEnv works
	os.Setenv("TEST_VAR_123", "test_value")
	defer os.Unsetenv("TEST_VAR_123")
	if app.GetEnv("TEST_VAR_123") != "test_value" {
		t.Error("DefaultApp() GetEnv doesn't work")
	}
}

func TestNewRootCmd(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	if cmd.Use != "imggen [prompt]" {
		t.Errorf("Use = %s, want 'imggen [prompt]'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"model", "size", "quality", "count", "output", "format", "style", "transparent", "api-key", "show"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not found", name)
		}
	}

	// Check short flags
	shortFlags := map[string]string{
		"m": "model",
		"s": "size",
		"q": "quality",
		"n": "count",
		"o": "output",
		"f": "format",
		"t": "transparent",
		"S": "show",
	}
	for short, long := range shortFlags {
		flag := cmd.Flags().ShorthandLookup(short)
		if flag == nil {
			t.Errorf("short flag -%s not found", short)
			continue
		}
		if flag.Name != long {
			t.Errorf("short flag -%s maps to %s, want %s", short, flag.Name, long)
		}
	}
}

func TestRunGenerate_NoAPIKey(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	// Use empty temp dir to ensure no stored key is found
	tmpDir := t.TempDir()
	t.Setenv("IMGGEN_CONFIG_DIR", tmpDir)
	// Clear the env var to ensure no key is found from environment
	t.Setenv("OPENAI_API_KEY", "")

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for missing API key")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("runGenerate() error = %v, want API key error", err)
	}
}

func TestRunGenerate_APIKeyFromFlag(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagAPIKey = "test-api-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestRunGenerate_APIKeyFromEnv(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	// Use empty config dir so no stored key is found
	configDir := t.TempDir()
	t.Setenv("IMGGEN_CONFIG_DIR", configDir)
	// Set the actual env var (keys.GetAPIKey uses os.Getenv directly)
	t.Setenv("OPENAI_API_KEY", "env-api-key")

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestRunGenerate_InvalidFormat(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagFormat = "gif"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("runGenerate() error = %v, want format error", err)
	}
}

func TestRunGenerate_UnknownModel(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "unknown-model"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for unknown model")
	}
	if !strings.Contains(err.Error(), "unknown model") {
		t.Errorf("runGenerate() error = %v, want unknown model error", err)
	}
}

func TestRunGenerate_ValidationError(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "dall-e-3"
	flagCount = 5 // DALL-E 3 only supports 1 image
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "invalid request") {
		t.Errorf("runGenerate() error = %v, want validation error", err)
	}
}

func TestRunGenerate_ProviderError(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return nil, errors.New("provider creation failed")
	}
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for provider failure")
	}
	if !strings.Contains(err.Error(), "failed to create provider") {
		t.Errorf("runGenerate() error = %v, want provider error", err)
	}
}

func TestRunGenerate_GenerationError(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return &mockProvider{
			generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
				return nil, errors.New("generation failed")
			},
		}, nil
	}
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for generation failure")
	}
	if !strings.Contains(err.Error(), "generation failed") {
		t.Errorf("runGenerate() error = %v, want generation error", err)
	}
}

func TestRunGenerate_SaveError(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return &mockProvider{
			generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
				return &models.Response{
					Images: []models.GeneratedImage{
						{Index: 0}, // No data, will fail to save
					},
				}, nil
			},
		}, nil
	}
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for save failure")
	}
}

func TestRunGenerate_SuccessWithOutput(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	flagOutput = filepath.Join(tmpDir, "output.png")

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}

	// Verify output file exists
	if _, err := os.Stat(flagOutput); os.IsNotExist(err) {
		t.Error("output file was not created")
	}

	// Verify output messages
	output := out.String()
	if !strings.Contains(output, "Generating") {
		t.Error("output missing 'Generating' message")
	}
	if !strings.Contains(output, "Saved:") {
		t.Error("output missing 'Saved:' message")
	}
	if !strings.Contains(output, "Done!") {
		t.Error("output missing 'Done!' message")
	}
}

func TestRunGenerate_SuccessWithRevisedPrompt(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return &mockProvider{
			generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
				return &models.Response{
					Images: []models.GeneratedImage{
						{Data: []byte("data"), Index: 0},
					},
					RevisedPrompt: "enhanced prompt",
				}, nil
			},
		}, nil
	}
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "Revised prompt: enhanced prompt") {
		t.Error("output missing revised prompt")
	}
}

func TestRunGenerate_MultipleImages(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return &mockProvider{
			generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
				return &models.Response{
					Images: []models.GeneratedImage{
						{Data: []byte("img1"), Index: 0},
						{Data: []byte("img2"), Index: 1},
						{Data: []byte("img3"), Index: 2},
					},
				}, nil
			},
		}, nil
	}
	flagAPIKey = "test-key"
	flagCount = 3

	tmpDir := t.TempDir()
	flagOutput = filepath.Join(tmpDir, "batch.png")

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}

	output := out.String()
	savedCount := strings.Count(output, "Saved:")
	if savedCount != 3 {
		t.Errorf("expected 3 'Saved:' messages, got %d", savedCount)
	}
}

func TestRunGenerate_InvalidSize(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagSize = "9999x9999"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want size validation error")
	}
}

func TestRunGenerate_InvalidQuality(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagQuality = "ultra-mega-hd"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want quality validation error")
	}
}

func TestRunGenerate_StyleNotSupported(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagStyle = "vivid"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want style validation error")
	}
}

func TestRunGenerate_TransparencyNotSupported(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "dall-e-3"
	flagTransparent = true
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want transparency validation error")
	}
}

func TestRunGenerate_TransparencyInvalidFormat(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagTransparent = true
	flagFormat = "jpeg"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want format validation error")
	}
}

func TestRunGenerate_CountZero(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagCount = 0
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for count=0")
	}
}

func TestRunGenerate_NegativeCount(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagCount = -5
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Fatal("runGenerate() error = nil, want error for negative count")
	}
}

func TestRun(t *testing.T) {
	// Test that run() returns error for missing args
	os.Args = []string{"imggen"} // No prompt argument
	err := run()
	if err == nil {
		t.Fatal("run() error = nil, want error for missing args")
	}
}

func TestVersion(t *testing.T) {
	if version == "" {
		t.Error("version variable is empty")
	}
	if commit == "" {
		t.Error("commit variable is empty")
	}
}

func TestRootCmd_Version(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	if cmd.Version == "" {
		t.Error("cmd.Version is empty")
	}
	if !strings.Contains(cmd.Version, version) {
		t.Errorf("cmd.Version = %s, want to contain %s", cmd.Version, version)
	}
}

func TestRootCmd_Args(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	// Should require exactly 1 argument
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("Args() error = nil, want error for no args")
	}
	if err := cmd.Args(cmd, []string{"prompt"}); err != nil {
		t.Errorf("Args() error = %v, want nil for single arg", err)
	}
	if err := cmd.Args(cmd, []string{"prompt1", "prompt2"}); err == nil {
		t.Error("Args() error = nil, want error for multiple args")
	}
}

func TestRootCmd_FlagDefaults(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	tests := []struct {
		flag   string
		defVal string
	}{
		{"model", "gpt-image-1"},
		{"size", ""},
		{"quality", ""},
		{"count", "1"},
		{"output", ""},
		{"format", "png"},
		{"style", ""},
		{"transparent", "false"},
		{"api-key", ""},
		{"show", "false"},
	}

	for _, tt := range tests {
		f := cmd.Flags().Lookup(tt.flag)
		if f == nil {
			t.Errorf("flag %s not found", tt.flag)
			continue
		}
		if f.DefValue != tt.defVal {
			t.Errorf("flag %s default = %s, want %s", tt.flag, f.DefValue, tt.defVal)
		}
	}
}

func TestRunGenerate_DallE3WithStyle(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "dall-e-3"
	flagStyle = "vivid"
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestRunGenerate_GPTImage1WithTransparency(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagTransparent = true
	flagFormat = "png"
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestRunGenerate_WithQuality(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagQuality = "high"
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestRunGenerate_WithSize(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "gpt-image-1"
	flagSize = "1024x1024"
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestRunGenerate_DallE2(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagModel = "dall-e-2"
	flagAPIKey = "test-key"

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}
}

func TestApp_DefaultNewProvider(t *testing.T) {
	app := DefaultApp()
	registry := models.DefaultRegistry()

	// Test with valid config
	cfg := &provider.Config{APIKey: "test-key"}
	prov, err := app.NewProvider(cfg, registry)
	if err != nil {
		t.Errorf("NewProvider() error = %v", err)
	}
	if prov == nil {
		t.Error("NewProvider() returned nil")
	}
}

func TestApp_DefaultNewSaver(t *testing.T) {
	app := DefaultApp()
	saver := app.NewSaver()
	if saver == nil {
		t.Error("NewSaver() returned nil")
	}
}

func TestApp_DefaultNewDisplayer(t *testing.T) {
	app := DefaultApp()
	var buf bytes.Buffer
	displayer := app.NewDisplayer(&buf)
	if displayer == nil {
		t.Error("NewDisplayer() returned nil")
	}
}

func TestRunGenerate_WithShowFlag(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagAPIKey = "test-key"
	flagShow = true

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "Saved:") {
		t.Error("output missing 'Saved:' message")
	}
	if !strings.Contains(output, "\x1b_G") {
		t.Error("output missing Kitty graphics escape sequence")
	}
}

func TestRunGenerate_ShowFlagDisplaysMultipleImages(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	app.NewProvider = func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
		return &mockProvider{
			generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
				return &models.Response{
					Images: []models.GeneratedImage{
						{Data: []byte("img1"), Index: 0},
						{Data: []byte("img2"), Index: 1},
					},
				}, nil
			},
		}, nil
	}
	flagAPIKey = "test-key"
	flagShow = true
	flagCount = 2

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}

	output := out.String()
	escCount := strings.Count(output, "\x1b_G")
	if escCount != 2 {
		t.Errorf("expected 2 Kitty escape sequences, got %d", escCount)
	}
}

func TestRunGenerate_WithoutShowFlag(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagAPIKey = "test-key"
	flagShow = false

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cmd := &cobra.Command{}
	err := runGenerate(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runGenerate() error = %v, want nil", err)
	}

	output := out.String()
	if strings.Contains(output, "\x1b_G") {
		t.Error("output should not contain Kitty escape sequence when --show is false")
	}
}

// Cost command tests

func TestNewCostCmd(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newCostCmd(app)

	if cmd.Use != "cost [today|week|month|total|provider]" {
		t.Errorf("Use = %s, want 'cost [today|week|month|total|provider]'", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short description is empty")
	}
}

func TestRunCost_Total(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Add some cost entries
	ctx := context.Background()
	store.LogCost(ctx, &session.CostEntry{
		Provider:   "openai",
		Model:      "gpt-image-1",
		Cost:       0.042,
		ImageCount: 1,
		Timestamp:  time.Now(),
	})
	store.Close()

	// Override getDBPath for testing
	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{"total"})
	if err != nil {
		t.Errorf("runCost() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Total cost:") {
		t.Error("output missing 'Total cost:'")
	}
	if !strings.Contains(output, "$0.0420") {
		t.Error("output missing cost amount")
	}
}

func TestRunCost_Today(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()
	store.LogCost(ctx, &session.CostEntry{
		Provider:   "openai",
		Model:      "gpt-image-1",
		Cost:       0.011,
		ImageCount: 1,
		Timestamp:  time.Now(),
	})
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{"today"})
	if err != nil {
		t.Errorf("runCost() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Today's cost:") {
		t.Error("output missing 'Today's cost:'")
	}
}

func TestRunCost_Week(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{"week"})
	if err != nil {
		t.Errorf("runCost() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "This week's cost:") {
		t.Error("output missing 'This week's cost:'")
	}
}

func TestRunCost_Month(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{"month"})
	if err != nil {
		t.Errorf("runCost() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "This month's cost:") {
		t.Error("output missing 'This month's cost:'")
	}
}

func TestRunCost_Provider(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()
	store.LogCost(ctx, &session.CostEntry{
		Provider:   "openai",
		Model:      "gpt-image-1",
		Cost:       0.042,
		ImageCount: 1,
		Timestamp:  time.Now(),
	})
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{"provider"})
	if err != nil {
		t.Errorf("runCost() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Provider") {
		t.Error("output missing 'Provider' header")
	}
	if !strings.Contains(output, "openai") {
		t.Error("output missing 'openai' provider")
	}
	if !strings.Contains(output, "Total") {
		t.Error("output missing 'Total' row")
	}
}

func TestRunCost_DefaultsToTotal(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{})
	if err != nil {
		t.Errorf("runCost() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Total cost:") {
		t.Error("default should show total cost")
	}
}

func TestRunCost_UnknownSubcommand(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runCost(app, []string{"unknown"})
	if err == nil {
		t.Error("runCost() error = nil, want error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Errorf("error = %v, want 'unknown subcommand'", err)
	}
}

// DB command tests

func TestNewDBCmd(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newDBCmd(app)

	if cmd.Use != "db" {
		t.Errorf("Use = %s, want 'db'", cmd.Use)
	}

	// Check subcommands exist
	infoCmd, _, err := cmd.Find([]string{"info"})
	if err != nil || infoCmd.Use != "info" {
		t.Error("'info' subcommand not found")
	}

	resetCmd, _, err := cmd.Find([]string{"reset"})
	if err != nil || resetCmd.Use != "reset" {
		t.Error("'reset' subcommand not found")
	}

	// Check --backup flag on reset
	backupFlag := resetCmd.Flags().Lookup("backup")
	if backupFlag == nil {
		t.Error("'--backup' flag not found on reset command")
	}
}

func TestRunDBInfo_NoDatabase(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err := runDBInfo(app)
	if err != nil {
		t.Errorf("runDBInfo() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Database location:") {
		t.Error("output missing database location")
	}
	if !strings.Contains(output, "does not exist") {
		t.Error("output should indicate database doesn't exist")
	}
}

func TestRunDBInfo_WithDatabase(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()
	store.LogCost(ctx, &session.CostEntry{
		Provider:   "openai",
		Model:      "gpt-image-1",
		Cost:       0.042,
		ImageCount: 1,
		Timestamp:  time.Now(),
	})
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runDBInfo(app)
	if err != nil {
		t.Errorf("runDBInfo() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Database location:") {
		t.Error("output missing database location")
	}
	if !strings.Contains(output, "Database size:") {
		t.Error("output missing database size")
	}
	if !strings.Contains(output, "Statistics:") {
		t.Error("output missing statistics")
	}
	if !strings.Contains(output, "Sessions:") {
		t.Error("output missing session count")
	}
	if !strings.Contains(output, "Total cost:") {
		t.Error("output missing total cost")
	}
}

func TestRunDBReset_NoDatabase(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err := runDBReset(app)
	if err != nil {
		t.Errorf("runDBReset() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "nothing to reset") {
		t.Error("output should indicate nothing to reset")
	}
}

func TestRunDBReset_WithDatabase(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runDBReset(app)
	if err != nil {
		t.Errorf("runDBReset() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Database deleted successfully") {
		t.Error("output should confirm deletion")
	}
	if !strings.Contains(output, "Fresh database created") {
		t.Error("output should confirm new database creation")
	}

	// Verify database was recreated
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database was not recreated")
	}
}

func TestRunDBReset_WithBackup(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)
	flagDBBackup = true

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	ctx := context.Background()
	store.LogCost(ctx, &session.CostEntry{
		Provider:   "openai",
		Model:      "gpt-image-1",
		Cost:       0.042,
		ImageCount: 1,
		Timestamp:  time.Now(),
	})
	store.Close()

	oldGetDBPath := getDBPath
	getDBPath = func() (string, error) { return dbPath, nil }
	defer func() { getDBPath = oldGetDBPath }()

	err = runDBReset(app)
	if err != nil {
		t.Errorf("runDBReset() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Backup saved to:") {
		t.Error("output should confirm backup")
	}

	// Check backup file was created
	files, _ := filepath.Glob(filepath.Join(tmpDir, "test.db.backup-*"))
	if len(files) == 0 {
		t.Error("backup file was not created")
	}
}

func TestGetDBPath(t *testing.T) {
	path, err := getDBPath()
	if err != nil {
		t.Errorf("getDBPath() error = %v", err)
	}
	if !strings.Contains(path, ".imggen") {
		t.Error("path should contain .imggen directory")
	}
	if !strings.HasSuffix(path, "sessions.db") {
		t.Error("path should end with sessions.db")
	}
}

func TestRootCmd_HasCostSubcommand(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	costCmd, _, err := cmd.Find([]string{"cost"})
	if err != nil {
		t.Errorf("cost subcommand not found: %v", err)
	}
	if costCmd == nil {
		t.Error("cost subcommand is nil")
	}
}

func TestRootCmd_HasDBSubcommand(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	dbCmd, _, err := cmd.Find([]string{"db"})
	if err != nil {
		t.Errorf("db subcommand not found: %v", err)
	}
	if dbCmd == nil {
		t.Error("db subcommand is nil")
	}
}

// Register command tests

func TestNewRegisterCmd(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRegisterCmd(app)

	if cmd.Use != "register [integration...]" {
		t.Errorf("Use = %s, want 'register [integration...]'", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short description is empty")
	}

	// Check flags exist
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("--dry-run flag not found")
	}
	if cmd.Flags().Lookup("force") == nil {
		t.Error("--force flag not found")
	}
	if cmd.Flags().Lookup("all") == nil {
		t.Error("--all flag not found")
	}
}

func TestRootCmd_HasRegisterSubcommand(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	registerCmd, _, err := cmd.Find([]string{"register"})
	if err != nil {
		t.Errorf("register subcommand not found: %v", err)
	}
	if registerCmd == nil {
		t.Error("register subcommand is nil")
	}
}

func TestRegisterCmd_HasSubcommands(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRegisterCmd(app)

	// Check subcommands exist
	subcommands := []string{"status", "unregister", "backups", "rollback"}
	for _, name := range subcommands {
		subCmd, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Errorf("%s subcommand not found: %v", name, err)
			continue
		}
		if subCmd == nil {
			t.Errorf("%s subcommand is nil", name)
		}
	}
}

func TestRunRegister_NoArgs(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runRegister(app, []string{})
	if err != nil {
		t.Errorf("runRegister() error = %v, want nil for no args (shows help)", err)
	}

	output := out.String()
	if !strings.Contains(output, "Available integrations:") {
		t.Error("output should show available integrations")
	}
}

func TestRunRegister_InvalidIntegration(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runRegister(app, []string{"invalid-integration"})
	if err == nil {
		t.Error("runRegister() error = nil, want error for invalid integration")
	}
	if !strings.Contains(err.Error(), "unknown integration") {
		t.Errorf("error = %v, want 'unknown integration'", err)
	}
}

func TestRunRegister_DryRun(t *testing.T) {
	resetFlags()
	flagRegisterDryRun = true
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runRegister(app, []string{"claude"})
	if err != nil {
		t.Errorf("runRegister() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "DRY RUN") {
		t.Error("output should indicate dry run mode")
	}
	// Should show either "dry-run mode" or "already registered" skip reason
	if !strings.Contains(output, "skipped") && !strings.Contains(output, "Summary:") {
		t.Error("output should show summary with skip status")
	}
}

func TestRunRegisterStatus(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runRegisterStatus(app)
	if err != nil {
		t.Errorf("runRegisterStatus() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Registration Status:") {
		t.Error("output should show registration status header")
	}
	if !strings.Contains(output, "Claude Code") {
		t.Error("output should list Claude Code")
	}
	if !strings.Contains(output, "Codex") {
		t.Error("output should list Codex")
	}
	if !strings.Contains(output, "Cursor") {
		t.Error("output should list Cursor")
	}
	if !strings.Contains(output, "Gemini") {
		t.Error("output should list Gemini")
	}
}

func TestRunUnregister_InvalidIntegration(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runUnregister(app, []string{"invalid"})
	if err == nil {
		t.Error("runUnregister() error = nil, want error for invalid integration")
	}
	if !strings.Contains(err.Error(), "unknown integration") {
		t.Errorf("error = %v, want 'unknown integration'", err)
	}
}

func TestRunListBackups_InvalidIntegration(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runListBackups(app, "invalid")
	if err == nil {
		t.Error("runListBackups() error = nil, want error for invalid integration")
	}
	if !strings.Contains(err.Error(), "unknown integration") {
		t.Errorf("error = %v, want 'unknown integration'", err)
	}
}

func TestRunListBackups_NoBackups(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	// This may or may not have backups depending on system state
	// but shouldn't error
	err := runListBackups(app, "claude")
	if err != nil {
		t.Errorf("runListBackups() error = %v", err)
	}
}

func TestRunRollback_NonexistentFile(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runRollback(app, "/nonexistent/path.backup-20240101-120000")
	if err == nil {
		t.Error("runRollback() error = nil, want error for nonexistent file")
	}
}

func TestRunRollback_InvalidPath(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runRollback(app, "/some/path/without/backup/suffix")
	if err == nil {
		t.Error("runRollback() error = nil, want error for invalid backup path format")
	}
}

// mockVideoProvider implements both provider.Provider and provider.VideoProvider for testing.
type mockVideoProvider struct {
	mockProvider
	generateVideoFunc func(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error)
}

func (m *mockVideoProvider) GenerateVideo(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	if m.generateVideoFunc != nil {
		return m.generateVideoFunc(ctx, req)
	}
	return &models.VideoResponse{
		Videos: []models.GeneratedVideo{
			{Data: []byte("test video data")},
		},
		Cost: &models.CostInfo{
			PerImage: 0.10,
			Total:    0.40,
			Currency: "USD",
		},
	}, nil
}

func (m *mockVideoProvider) SupportsVideoModel(model string) bool {
	return model == "sora-2" || model == "sora-2-pro"
}

func (m *mockVideoProvider) ListVideoModels() []string {
	return []string{"sora-2", "sora-2-pro"}
}

func newTestAppWithVideoProvider(out *bytes.Buffer, videoProv *mockVideoProvider) *App {
	return &App{
		Out:      out,
		Err:      out,
		Registry: models.DefaultRegistry(),
		GetEnv: func(key string) string {
			return ""
		},
		NewProvider: func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
			return videoProv, nil
		},
		NewSaver:     image.NewSaver,
		NewDisplayer: display.New,
	}
}

func TestNewVideoCmd(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newVideoCmd(app)

	if cmd.Use != "video <prompt>" {
		t.Errorf("Use = %s, want 'video <prompt>'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"model", "duration", "size", "output", "api-key", "verbose"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not found", name)
		}
	}

	// Check short flags
	shortFlags := map[string]string{
		"m": "model",
		"d": "duration",
		"s": "size",
		"o": "output",
		"v": "verbose",
	}
	for short, long := range shortFlags {
		flag := cmd.Flags().ShorthandLookup(short)
		if flag == nil {
			t.Errorf("short flag -%s not found", short)
		} else if flag.Name != long {
			t.Errorf("short flag -%s maps to %s, want %s", short, flag.Name, long)
		}
	}
}

func TestRunVideo_NoAPIKey(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := &App{
		Out:      out,
		Err:      out,
		Registry: models.DefaultRegistry(),
		GetEnv:   func(key string) string { return "" },
		NewProvider: func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
			return &mockVideoProvider{}, nil
		},
		NewSaver:     image.NewSaver,
		NewDisplayer: display.New,
	}

	// Override config dir to ensure no stored keys are found
	tmpDir := t.TempDir()
	os.Setenv("IMGGEN_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("IMGGEN_CONFIG_DIR")

	// Ensure OPENAI_API_KEY is not set
	origKey := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	defer func() {
		if origKey != "" {
			os.Setenv("OPENAI_API_KEY", origKey)
		}
	}()

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Error("runVideo() error = nil, want error for missing API key")
	}
}

func TestRunVideo_UnknownModel(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestAppWithVideoProvider(out, &mockVideoProvider{})

	flagVideoModel = "unknown-model"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Error("runVideo() error = nil, want error for unknown model")
	}
	if !strings.Contains(err.Error(), "unknown video model") {
		t.Errorf("error = %v, want error containing 'unknown video model'", err)
	}
}

func TestRunVideo_InvalidDuration(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestAppWithVideoProvider(out, &mockVideoProvider{})

	flagVideoModel = "sora-2"
	flagVideoDuration = 99 // Invalid duration
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Error("runVideo() error = nil, want error for invalid duration")
	}
	if !strings.Contains(err.Error(), "invalid request") {
		t.Errorf("error = %v, want error containing 'invalid request'", err)
	}
}

func TestRunVideo_Success(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-video.mp4")

	mockProv := &mockVideoProvider{}
	app := newTestAppWithVideoProvider(out, mockProv)

	flagVideoModel = "sora-2"
	flagVideoDuration = 4
	flagVideoOutput = outputPath
	flagAPIKey = "test-key"

	// Override getDBPath for test
	origGetDBPath := getDBPath
	getDBPath = func() (string, error) {
		return filepath.Join(tmpDir, "test.db"), nil
	}
	defer func() { getDBPath = origGetDBPath }()

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"a cat walking"}, app)

	if err != nil {
		t.Errorf("runVideo() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "Generating video") {
		t.Error("output should contain 'Generating video'")
	}
	if !strings.Contains(output, "Saved:") {
		t.Error("output should contain 'Saved:'")
	}
	if !strings.Contains(output, "Cost:") {
		t.Error("output should contain 'Cost:'")
	}
	if !strings.Contains(output, "Done!") {
		t.Error("output should contain 'Done!'")
	}

	// Check file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("video file was not created")
	}
}

func TestRunVideo_GenerationError(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}

	mockProv := &mockVideoProvider{
		generateVideoFunc: func(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
			return nil, errors.New("video generation failed")
		},
	}
	app := newTestAppWithVideoProvider(out, mockProv)

	flagVideoModel = "sora-2"
	flagAPIKey = "test-key"

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"test prompt"}, app)

	if err == nil {
		t.Error("runVideo() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "video generation failed") {
		t.Errorf("error = %v, want error containing 'video generation failed'", err)
	}
}

func TestRunVideo_WithDefaultFilename(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}

	tmpDir := t.TempDir()

	mockProv := &mockVideoProvider{}
	app := newTestAppWithVideoProvider(out, mockProv)

	// Change to temp dir so generated file goes there
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	flagVideoModel = "sora-2"
	flagVideoOutput = "" // No output specified, should generate filename
	flagAPIKey = "test-key"

	// Override getDBPath for test
	origGetDBPath := getDBPath
	getDBPath = func() (string, error) {
		return filepath.Join(tmpDir, "test.db"), nil
	}
	defer func() { getDBPath = origGetDBPath }()

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"test prompt"}, app)

	if err != nil {
		t.Errorf("runVideo() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "video-") && !strings.Contains(output, ".mp4") {
		t.Error("output should contain generated filename with video- prefix and .mp4 extension")
	}
}

func TestRunVideo_Sora2Pro(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "pro-video.mp4")

	mockProv := &mockVideoProvider{
		generateVideoFunc: func(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
			if req.Model != "sora-2-pro" {
				t.Errorf("expected model sora-2-pro, got %s", req.Model)
			}
			if req.Duration != 8 {
				t.Errorf("expected duration 8, got %d", req.Duration)
			}
			return &models.VideoResponse{
				Videos: []models.GeneratedVideo{{Data: []byte("pro video data")}},
				Cost: &models.CostInfo{
					PerImage: 0.30,
					Total:    2.40,
					Currency: "USD",
				},
			}, nil
		},
	}
	app := newTestAppWithVideoProvider(out, mockProv)

	flagVideoModel = "sora-2-pro"
	flagVideoDuration = 8
	flagVideoOutput = outputPath
	flagAPIKey = "test-key"

	// Override getDBPath for test
	origGetDBPath := getDBPath
	getDBPath = func() (string, error) {
		return filepath.Join(tmpDir, "test.db"), nil
	}
	defer func() { getDBPath = origGetDBPath }()

	cmd := &cobra.Command{}
	err := runVideo(cmd, []string{"cinematic sunset"}, app)

	if err != nil {
		t.Errorf("runVideo() error = %v, want nil", err)
	}

	output := out.String()
	if !strings.Contains(output, "sora-2-pro") {
		t.Error("output should contain model name sora-2-pro")
	}
}

func TestRootCmd_HasVideoSubcommand(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	var hasVideo bool
	for _, subcmd := range cmd.Commands() {
		if subcmd.Name() == "video" {
			hasVideo = true
			break
		}
	}

	if !hasVideo {
		t.Error("root command should have 'video' subcommand")
	}
}
