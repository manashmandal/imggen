package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/manash/imggen/internal/display"
	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/internal/provider/openai"
	"github.com/manash/imggen/internal/repl"
	"github.com/manash/imggen/internal/session"
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
	flagShow        bool
	flagInteractive bool
)

type App struct {
	Out          io.Writer
	Err          io.Writer
	Registry     *models.ModelRegistry
	GetEnv       func(string) string
	NewProvider  func(cfg *provider.Config, registry *models.ModelRegistry) (provider.Provider, error)
	NewSaver     func() *image.Saver
	NewDisplayer func(io.Writer) *display.Displayer
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
		NewSaver:     image.NewSaver,
		NewDisplayer: display.New,
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
  imggen -m gpt-image-1 -n 3 --transparent "logo design"
  imggen -i  # start interactive mode`,
		Args: func(cmd *cobra.Command, args []string) error {
			if flagInteractive {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		Version: fmt.Sprintf("%s (commit: %s)", version, commit),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagInteractive {
				return runInteractive(cmd, app)
			}
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
	cmd.Flags().BoolVarP(&flagShow, "show", "S", false, "display image in terminal (Kitty graphics protocol)")
	cmd.Flags().BoolVarP(&flagInteractive, "interactive", "i", false, "start interactive editing mode")

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

	if flagShow {
		if !display.IsTerminalSupported() {
			fmt.Fprintln(app.Err, "Warning: terminal may not support Kitty graphics protocol")
		}
		displayer := app.NewDisplayer(app.Out)
		if err := displayer.DisplayAll(ctx, resp); err != nil {
			fmt.Fprintf(app.Err, "Warning: failed to display image: %v\n", err)
		}
	}

	if resp.RevisedPrompt != "" {
		fmt.Fprintf(app.Out, "Revised prompt: %s\n", resp.RevisedPrompt)
	}

	fmt.Fprintln(app.Out, "Done!")
	return nil
}

func runInteractive(_ *cobra.Command, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	apiKey := flagAPIKey
	if apiKey == "" {
		apiKey = app.GetEnv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("API key required: set OPENAI_API_KEY or use --api-key")
	}

	prov, err := app.NewProvider(&provider.Config{APIKey: apiKey}, app.Registry)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	store, err := session.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	defer store.Close()

	sessionMgr := session.NewManager(store, flagModel)

	replCfg := &repl.Config{
		In:         os.Stdin,
		Out:        app.Out,
		Err:        app.Err,
		Provider:   prov,
		Registry:   app.Registry,
		SessionMgr: sessionMgr,
		Displayer:  app.NewDisplayer(app.Out),
		Saver:      app.NewSaver(),
	}

	r := repl.New(replCfg)
	return r.Run(ctx)
}
