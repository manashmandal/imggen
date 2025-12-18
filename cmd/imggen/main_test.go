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
	app.GetEnv = func(key string) string {
		if key == "OPENAI_API_KEY" {
			return "env-api-key"
		}
		return ""
	}

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

func TestRootCmd_HasPriceSubcommand(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)
	cmd := newRootCmd(app)

	priceCmd, _, err := cmd.Find([]string{"price"})
	if err != nil {
		t.Errorf("price subcommand not found: %v", err)
	}
	if priceCmd == nil {
		t.Error("price subcommand is nil")
	}
}

func TestRunPriceShow(t *testing.T) {
	resetFlags()
	out := &bytes.Buffer{}
	app := newTestApp(out)

	err := runPriceShow(app)
	if err != nil {
		t.Errorf("runPriceShow() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "gpt-image-1") {
		t.Error("output should contain gpt-image-1")
	}
	if !strings.Contains(output, "dall-e-3") {
		t.Error("output should contain dall-e-3")
	}
	if !strings.Contains(output, "dall-e-2") {
		t.Error("output should contain dall-e-2")
	}
}

func TestShowBuiltinPricing(t *testing.T) {
	out := &bytes.Buffer{}
	app := newTestApp(out)

	showBuiltinPricing(app)

	output := out.String()
	if !strings.Contains(output, "gpt-image-1") {
		t.Error("output should contain gpt-image-1")
	}
	if !strings.Contains(output, "dall-e-3") {
		t.Error("output should contain dall-e-3")
	}
	if !strings.Contains(output, "dall-e-2") {
		t.Error("output should contain dall-e-2")
	}
	if !strings.Contains(output, "$0.0110") {
		t.Error("output should show gpt-image-1 low quality price")
	}
	if !strings.Contains(output, "$0.0400") {
		t.Error("output should show dall-e-3 standard price")
	}
}
