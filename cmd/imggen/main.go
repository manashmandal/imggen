package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/internal/provider/openai"
	"github.com/manash/imggen/pkg/models"
)

var (
	version = "dev"
	commit  = "none"
)

var (
	flagModel       string
	flagSize        string
	flagQuality     string
	flagCount       int
	flagOutput      string
	flagFormat      string
	flagStyle       string
	flagTransparent bool
	flagAPIKey      string
)

type App struct {
	Out         io.Writer
	Err         io.Writer
	Registry    *models.ModelRegistry
	GetEnv      func(string) string
	NewProvider func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error)
	NewSaver    func() *image.Saver
}

func DefaultApp() *App {
	return &App{
		Out:      os.Stdout,
		Err:      os.Stderr,
		Registry: models.DefaultRegistry(),
		GetEnv:   os.Getenv,
		NewProvider: func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error) {
			return openai.New(cfg, registry)
		},
		NewSaver: image.NewSaver,
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	app := DefaultApp()
	rootCmd := newRootCmd(app)
	return rootCmd.Execute()
}

func newRootCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "imggen [prompt]",
		Short: "Generate images using AI image generation APIs",
		Long: `imggen is a CLI tool for generating images using AI image generation APIs.

Supported providers:
  - OpenAI (gpt-image-1, dall-e-3, dall-e-2)
  - Stability AI (coming soon)

Examples:
  imggen "a sunset over mountains"
  imggen -m dall-e-3 -s 1792x1024 -q hd "panoramic cityscape"
  imggen -m gpt-image-1 -n 3 --transparent "logo design"`,
		Args:    cobra.ExactArgs(1),
		Version: fmt.Sprintf("%s (commit: %s)", version, commit),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd, args, app)
		},
	}

	cmd.Flags().StringVarP(&flagModel, "model", "m", "gpt-image-1", "model to use (gpt-image-1, dall-e-3, dall-e-2)")
	cmd.Flags().StringVarP(&flagSize, "size", "s", "", "image size (e.g., 1024x1024)")
	cmd.Flags().StringVarP(&flagQuality, "quality", "q", "", "quality level")
	cmd.Flags().IntVarP(&flagCount, "count", "n", 1, "number of images to generate")
	cmd.Flags().StringVarP(&flagOutput, "output", "o", "", "output filename")
	cmd.Flags().StringVarP(&flagFormat, "format", "f", "png", "output format (png, jpeg, webp)")
	cmd.Flags().StringVar(&flagStyle, "style", "", "style for dall-e-3 (vivid, natural)")
	cmd.Flags().BoolVarP(&flagTransparent, "transparent", "t", false, "transparent background (gpt-image-1 only)")
	cmd.Flags().StringVar(&flagAPIKey, "api-key", "", "API key (defaults to OPENAI_API_KEY)")

	return cmd
}

func runGenerate(_ *cobra.Command, args []string, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	prompt := args[0]

	apiKey := flagAPIKey
	if apiKey == "" {
		apiKey = app.GetEnv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("API key required: set OPENAI_API_KEY or use --api-key")
	}

	format := models.OutputFormat(flagFormat)
	if !format.IsValid() {
		return fmt.Errorf("invalid format %q: must be one of %v", flagFormat, models.ValidFormats())
	}

	req := models.NewRequest(prompt)
	req.Model = flagModel
	req.Size = flagSize
	req.Quality = flagQuality
	req.Count = flagCount
	req.Style = flagStyle
	req.Format = format
	req.Transparent = flagTransparent

	caps, ok := app.Registry.Get(flagModel)
	if !ok {
		return fmt.Errorf("unknown model %q: available models: %v", flagModel, app.Registry.List())
	}

	caps.ApplyDefaults(req)

	if err := caps.Validate(req); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	providerCfg := &provider.Config{APIKey: apiKey}
	prov, err := app.NewProvider(providerCfg, app.Registry)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	fmt.Fprintf(app.Out, "Generating %d image(s) with %s...\n", req.Count, req.Model)

	resp, err := prov.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	saver := app.NewSaver()
	paths, err := saver.SaveAll(ctx, resp, flagOutput, format)
	if err != nil {
		return err
	}

	for _, path := range paths {
		fmt.Fprintf(app.Out, "Saved: %s\n", path)
	}

	if resp.RevisedPrompt != "" {
		fmt.Fprintf(app.Out, "Revised prompt: %s\n", resp.RevisedPrompt)
	}

	fmt.Fprintln(app.Out, "Done!")
	return nil
}
