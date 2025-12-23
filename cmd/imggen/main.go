package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/manash/imggen/internal/batch"
	"github.com/manash/imggen/internal/display"
	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/keys"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/internal/provider/openai"
	"github.com/manash/imggen/internal/register"
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
	flagVerbose     bool
	flagPrompts     []string
	flagParallel    int
)

var (
	flagBatchOutput      string
	flagBatchModel       string
	flagBatchSize        string
	flagBatchQuality     string
	flagBatchFormat      string
	flagBatchParallel    int
	flagBatchStopOnError bool
	flagBatchDelay       int
)

var (
	flagOCRModel         string
	flagOCRSchema        string
	flagOCRSchemaName    string
	flagOCRSuggestSchema bool
	flagOCRPrompt        string
	flagOCROutput        string
	flagOCRURL           string
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

Note: Only OpenAI is currently supported. Other providers (Stability AI, etc.) are work in progress.

Examples:
  imggen "a sunset over mountains"
  imggen -m dall-e-3 -s 1792x1024 -q hd "panoramic cityscape"
  imggen -m gpt-image-1 -n 3 --transparent "logo design"
  imggen --prompt "a sunset" --prompt "a cat" -o ./output
  imggen -i  # start interactive mode`,
		Args: func(cmd *cobra.Command, args []string) error {
			if flagInteractive {
				return nil
			}
			if len(flagPrompts) > 0 {
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
	cmd.Flags().StringVarP(&flagOutput, "output", "o", "", "output filename or directory (directory when using --prompt)")
	cmd.Flags().StringVarP(&flagFormat, "format", "f", "png", "output format (png, jpeg, webp)")
	cmd.Flags().StringVar(&flagStyle, "style", "", "style for dall-e-3 (vivid, natural)")
	cmd.Flags().BoolVarP(&flagTransparent, "transparent", "t", false, "transparent background (gpt-image-1 only)")
	cmd.Flags().StringVar(&flagAPIKey, "api-key", "", "API key (defaults to OPENAI_API_KEY)")
	cmd.Flags().BoolVarP(&flagShow, "show", "S", false, "display image in terminal (Kitty graphics protocol)")
	cmd.Flags().BoolVarP(&flagInteractive, "interactive", "i", false, "start interactive editing mode")
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log HTTP requests and responses (API keys redacted)")
	cmd.Flags().StringArrayVarP(&flagPrompts, "prompt", "P", nil, "prompt for image generation (can be specified multiple times)")
	cmd.Flags().IntVarP(&flagParallel, "parallel", "p", 1, "number of parallel workers for multiple prompts")

	cmd.AddCommand(newCostCmd(app))
	cmd.AddCommand(newDBCmd(app))
	cmd.AddCommand(newBatchCmd(app))
	cmd.AddCommand(newRegisterCmd(app))
	cmd.AddCommand(newKeysCmd(app))
	cmd.AddCommand(newOCRCmd(app))

	return cmd
}

func newCostCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost [today|week|month|total|provider]",
		Short: "View cost tracking information",
		Long: `View cost tracking information for image generation.

Subcommands:
  today     - Show today's costs
  week      - Show this week's costs (last 7 days)
  month     - Show this month's costs (last 30 days)
  total     - Show all-time total costs (default)
  provider  - Show costs broken down by provider

Examples:
  imggen cost           # show total costs
  imggen cost today     # show today's costs
  imggen cost provider  # show costs by provider`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCost(app, args)
		},
	}
	return cmd
}

func runCost(app *App, args []string) error {
	ctx := context.Background()

	dbPath, err := getDBPath()
	if err != nil {
		return err
	}

	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	subcommand := "total"
	if len(args) > 0 {
		subcommand = args[0]
	}

	fmt.Fprintln(app.Out, "\033[33mNote: Costs estimated from https://openai.com/api/pricing (not returned by API)\033[0m")
	fmt.Fprintln(app.Out)

	now := time.Now()

	switch subcommand {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end := start.Add(24 * time.Hour)
		summary, err := store.GetCostByDateRange(ctx, start, end)
		if err != nil {
			return fmt.Errorf("failed to get costs: %w", err)
		}
		fmt.Fprintf(app.Out, "Today's cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)

	case "week":
		start := now.AddDate(0, 0, -7)
		summary, err := store.GetCostByDateRange(ctx, start, now)
		if err != nil {
			return fmt.Errorf("failed to get costs: %w", err)
		}
		fmt.Fprintf(app.Out, "This week's cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)

	case "month":
		start := now.AddDate(0, 0, -30)
		summary, err := store.GetCostByDateRange(ctx, start, now)
		if err != nil {
			return fmt.Errorf("failed to get costs: %w", err)
		}
		fmt.Fprintf(app.Out, "This month's cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)

	case "total":
		summary, err := store.GetTotalCost(ctx)
		if err != nil {
			return fmt.Errorf("failed to get costs: %w", err)
		}
		fmt.Fprintf(app.Out, "Total cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)

	case "provider":
		summaries, err := store.GetCostByProvider(ctx)
		if err != nil {
			return fmt.Errorf("failed to get costs: %w", err)
		}
		fmt.Fprintf(app.Out, "%-12s %8s %10s\n", "Provider", "Images", "Cost")
		fmt.Fprintln(app.Out, "--------------------------------")
		var totalImages int
		var totalCost float64
		for _, s := range summaries {
			fmt.Fprintf(app.Out, "%-12s %8d %10s\n", s.Provider, s.ImageCount, fmt.Sprintf("$%.4f", s.TotalCost))
			totalImages += s.ImageCount
			totalCost += s.TotalCost
		}
		fmt.Fprintln(app.Out, "--------------------------------")
		fmt.Fprintf(app.Out, "%-12s %8d %10s\n", "Total", totalImages, fmt.Sprintf("$%.4f", totalCost))

	default:
		return fmt.Errorf("unknown subcommand %q: use today, week, month, total, or provider", subcommand)
	}

	return nil
}

func runGenerate(_ *cobra.Command, args []string, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Get API key using priority: --api-key flag > stored key > env var
	apiKey, _, err := keys.GetAPIKey(flagAPIKey, "openai", "OPENAI_API_KEY")
	if err != nil {
		return err
	}

	format := models.OutputFormat(flagFormat)
	if !format.IsValid() {
		return fmt.Errorf("invalid format %q: must be one of %v", flagFormat, models.ValidFormats())
	}

	// Handle multiple prompts via --prompt flag
	if len(flagPrompts) > 0 {
		return runMultiPrompt(ctx, app, apiKey, format)
	}

	// Single prompt mode (positional argument)
	prompt := args[0]

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

	providerCfg := &provider.Config{APIKey: apiKey, Verbose: flagVerbose}
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

	if resp.Cost != nil {
		fmt.Fprintf(app.Out, "Cost: $%.4f (%d image(s) @ $%.4f/image, %s %s %s)\n",
			resp.Cost.Total, len(resp.Images), resp.Cost.PerImage,
			req.Model, req.Size, req.Quality)

		// Log cost to database (empty strings for iteration/session as CLI mode doesn't have sessions)
		store, err := session.NewStore()
		if err == nil {
			defer store.Close()
			costEntry := &session.CostEntry{
				IterationID: "",
				SessionID:   "",
				Provider:    string(prov.Name()),
				Model:       req.Model,
				Cost:        resp.Cost.Total,
				ImageCount:  len(resp.Images),
				Timestamp:   time.Now(),
			}
			if logErr := store.LogCost(ctx, costEntry); logErr != nil {
				fmt.Fprintf(app.Err, "Warning: failed to log cost: %v\n", logErr)
			}
		}
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

func runMultiPrompt(ctx context.Context, app *App, apiKey string, format models.OutputFormat) error {
	outputDir := flagOutput
	if outputDir == "" {
		outputDir = "."
		fmt.Fprintln(app.Out, "\033[33mWarning: No output directory specified. Images will be saved to current directory.\033[0m")

		if isTerminal() {
			fmt.Fprint(app.Out, "Continue? [Y/n] ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "n" || response == "no" {
				fmt.Fprintln(app.Out, "Aborted.")
				return nil
			}
		}
	} else {
		if _, err := os.Stat(outputDir); os.IsNotExist(err) {
			if isTerminal() {
				fmt.Fprintf(app.Out, "Directory %q does not exist. Create it? [Y/n] ", outputDir)
				reader := bufio.NewReader(os.Stdin)
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response == "n" || response == "no" {
					fmt.Fprintln(app.Out, "Aborted.")
					return nil
				}
			}
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			fmt.Fprintf(app.Out, "Created directory: %s\n", outputDir)
		}
	}

	items := make([]batch.Item, len(flagPrompts))
	for i, prompt := range flagPrompts {
		items[i] = batch.Item{
			Index:  i + 1,
			Prompt: prompt,
		}
	}

	fmt.Fprintf(app.Out, "Generating %d images with %s\n", len(items), flagModel)
	fmt.Fprintf(app.Out, "Output directory: %s\n\n", outputDir)

	providerCfg := &provider.Config{APIKey: apiKey, Verbose: flagVerbose}
	prov, err := app.NewProvider(providerCfg, app.Registry)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	processor := batch.NewProcessor(prov, app.NewSaver(), app.Registry, app.Out, app.Err)

	opts := &batch.Options{
		OutputDir:      outputDir,
		DefaultModel:   flagModel,
		DefaultSize:    flagSize,
		DefaultQuality: flagQuality,
		Format:         format,
		Parallel:       flagParallel,
		StopOnError:    false,
		DelayMs:        0,
	}

	results, err := processor.Process(ctx, items, opts)

	processor.PrintSummary(results)

	if err != nil {
		return err
	}

	var totalCost float64
	for _, r := range results {
		if r.Error == nil {
			totalCost += r.Cost
		}
	}
	if totalCost > 0 {
		store, storeErr := session.NewStore()
		if storeErr == nil {
			defer store.Close()
			costEntry := &session.CostEntry{
				IterationID: "",
				SessionID:   "",
				Provider:    string(prov.Name()),
				Model:       flagModel,
				Cost:        totalCost,
				ImageCount:  countSuccessful(results),
				Timestamp:   time.Now(),
			}
			if logErr := store.LogCost(ctx, costEntry); logErr != nil {
				fmt.Fprintf(app.Err, "Warning: failed to log cost: %v\n", logErr)
			}
		}
	}

	return nil
}

func runInteractive(_ *cobra.Command, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Get API key using priority: --api-key flag > stored key > env var
	apiKey, _, err := keys.GetAPIKey(flagAPIKey, "openai", "OPENAI_API_KEY")
	if err != nil {
		return err
	}

	prov, err := app.NewProvider(&provider.Config{APIKey: apiKey, Verbose: flagVerbose}, app.Registry)
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

var flagDBBackup bool

func newDBCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management commands",
		Long:  `Manage the SQLite database storing sessions and cost data.`,
	}

	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show database location and statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBInfo(app)
		},
	}

	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset database (delete all data)",
		Long: `Reset the database by deleting all data.

Use --backup to save the old database before resetting.
The backup will be saved as sessions.db.backup-TIMESTAMP`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBReset(app)
		},
	}
	resetCmd.Flags().BoolVar(&flagDBBackup, "backup", false, "backup old database before reset")

	cmd.AddCommand(infoCmd)
	cmd.AddCommand(resetCmd)

	return cmd
}

func runDBInfo(app *App) error {
	ctx := context.Background()

	dbPath, err := getDBPath()
	if err != nil {
		return err
	}

	fmt.Fprintf(app.Out, "Database location: %s\n\n", dbPath)

	// Check if file exists
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		fmt.Fprintln(app.Out, "Database does not exist yet (will be created on first use)")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat database: %w", err)
	}

	fmt.Fprintf(app.Out, "Database size: %.2f KB\n", float64(info.Size())/1024)
	fmt.Fprintf(app.Out, "Last modified: %s\n\n", info.ModTime().Format("2006-01-02 15:04:05"))

	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	// Get session count
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Get cost summary
	costSummary, err := store.GetTotalCost(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cost summary: %w", err)
	}

	fmt.Fprintln(app.Out, "Statistics:")
	fmt.Fprintf(app.Out, "  Sessions: %d\n", len(sessions))
	fmt.Fprintf(app.Out, "  Total images generated: %d\n", costSummary.ImageCount)
	fmt.Fprintf(app.Out, "  Total cost: $%.4f\n", costSummary.TotalCost)

	return nil
}

func runDBReset(app *App) error {
	dbPath, err := getDBPath()
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(app.Out, "Database does not exist, nothing to reset")
		return nil
	}

	if flagDBBackup {
		backupPath := dbPath + ".backup-" + time.Now().Format("20060102-150405")
		data, err := os.ReadFile(dbPath)
		if err != nil {
			return fmt.Errorf("failed to read database for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write backup: %w", err)
		}
		fmt.Fprintf(app.Out, "Backup saved to: %s\n", backupPath)
	}

	if err := os.Remove(dbPath); err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}

	fmt.Fprintln(app.Out, "Database deleted successfully")

	// Create fresh database
	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create new database: %w", err)
	}
	store.Close()

	fmt.Fprintln(app.Out, "Fresh database created")
	return nil
}

var getDBPath = func() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".imggen", "sessions.db"), nil
}

func newBatchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch <input-file>",
		Short: "Generate images in batch from a file",
		Long: `Generate multiple images from a file containing prompts.

Input formats:
  .txt - One prompt per line (lines starting with # are ignored)
  .json - JSON array of objects with prompt/model/size/quality fields

Examples:
  imggen batch prompts.txt
  imggen batch prompts.txt -o ./output
  imggen batch prompts.json -o ./output -p 3
  imggen batch prompts.txt -o ./output -m dall-e-3 -q hd`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBatch(cmd, args, app)
		},
	}

	cmd.Flags().StringVarP(&flagBatchOutput, "output", "o", "", "output directory for generated images")
	cmd.Flags().StringVarP(&flagBatchModel, "model", "m", "gpt-image-1", "default model for prompts without model specified")
	cmd.Flags().StringVarP(&flagBatchSize, "size", "s", "", "default image size")
	cmd.Flags().StringVarP(&flagBatchQuality, "quality", "q", "", "default quality level")
	cmd.Flags().StringVarP(&flagBatchFormat, "format", "f", "png", "output format (png, jpeg, webp)")
	cmd.Flags().IntVarP(&flagBatchParallel, "parallel", "p", 1, "number of parallel workers (1 = sequential)")
	cmd.Flags().BoolVar(&flagBatchStopOnError, "stop-on-error", false, "stop batch on first error")
	cmd.Flags().IntVar(&flagBatchDelay, "delay", 0, "delay between requests in milliseconds")
	cmd.Flags().StringVar(&flagAPIKey, "api-key", "", "API key (defaults to OPENAI_API_KEY)")
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log HTTP requests and responses")

	return cmd
}

func runBatch(_ *cobra.Command, args []string, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	inputFile := args[0]

	// Get API key using priority: --api-key flag > stored key > env var
	apiKey, _, err := keys.GetAPIKey(flagAPIKey, "openai", "OPENAI_API_KEY")
	if err != nil {
		return err
	}

	format := models.OutputFormat(flagBatchFormat)
	if !format.IsValid() {
		return fmt.Errorf("invalid format %q: must be one of %v", flagBatchFormat, models.ValidFormats())
	}

	items, err := batch.ParseFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to parse input file: %w", err)
	}

	fmt.Fprintf(app.Out, "Batch generation: %d prompts\n", len(items))

	outputDir := flagBatchOutput
	if outputDir == "" {
		outputDir = "."
		fmt.Fprintln(app.Out, "\033[33mWarning: No output directory specified. Images will be saved to current directory.\033[0m")

		if isTerminal() {
			fmt.Fprint(app.Out, "Continue? [Y/n] ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "n" || response == "no" {
				fmt.Fprintln(app.Out, "Aborted.")
				return nil
			}
		}
	} else {
		if _, err := os.Stat(outputDir); os.IsNotExist(err) {
			if isTerminal() {
				fmt.Fprintf(app.Out, "Directory %q does not exist. Create it? [Y/n] ", outputDir)
				reader := bufio.NewReader(os.Stdin)
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response == "n" || response == "no" {
					fmt.Fprintln(app.Out, "Aborted.")
					return nil
				}
			}
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			fmt.Fprintf(app.Out, "Created directory: %s\n", outputDir)
		}
	}

	fmt.Fprintf(app.Out, "Output directory: %s\n\n", outputDir)

	providerCfg := &provider.Config{APIKey: apiKey, Verbose: flagVerbose}
	prov, err := app.NewProvider(providerCfg, app.Registry)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	processor := batch.NewProcessor(prov, app.NewSaver(), app.Registry, app.Out, app.Err)

	opts := &batch.Options{
		OutputDir:      outputDir,
		DefaultModel:   flagBatchModel,
		DefaultSize:    flagBatchSize,
		DefaultQuality: flagBatchQuality,
		Format:         format,
		Parallel:       flagBatchParallel,
		StopOnError:    flagBatchStopOnError,
		DelayMs:        flagBatchDelay,
	}

	results, err := processor.Process(ctx, items, opts)

	processor.PrintSummary(results)

	if err != nil {
		return err
	}

	var totalCost float64
	for _, r := range results {
		if r.Error == nil {
			totalCost += r.Cost
		}
	}
	if totalCost > 0 {
		store, storeErr := session.NewStore()
		if storeErr == nil {
			defer store.Close()
			costEntry := &session.CostEntry{
				IterationID: "",
				SessionID:   "",
				Provider:    string(prov.Name()),
				Model:       flagBatchModel,
				Cost:        totalCost,
				ImageCount:  countSuccessful(results),
				Timestamp:   time.Now(),
			}
			if logErr := store.LogCost(ctx, costEntry); logErr != nil {
				fmt.Fprintf(app.Err, "Warning: failed to log cost: %v\n", logErr)
			}
		}
	}

	return nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func countSuccessful(results []batch.Result) int {
	count := 0
	for _, r := range results {
		if r.Error == nil {
			count++
		}
	}
	return count
}

var (
	flagRegisterDryRun bool
	flagRegisterForce  bool
)

func newRegisterCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register [integration...]",
		Short: "Register imggen with AI CLI tools",
		Long: `Register imggen with AI CLI tools so they know how to use it.

Supported integrations:
  claude  - Claude Code (~/.claude/skills/imggen/SKILL.md)
  codex   - OpenAI Codex CLI (~/.codex/AGENTS.md)
  cursor  - Cursor (~/.cursor/rules/imggen.mdc)
  gemini  - Gemini CLI (~/.gemini/GEMINI.md)

Examples:
  imggen register --all              # Register with all supported CLIs
  imggen register claude codex       # Register with specific CLIs
  imggen register --dry-run --all    # Preview what would happen
  imggen register status             # Show registration status
  imggen register unregister claude  # Remove from Claude Code

The command will:
  1. Show what changes will be made
  2. Create a backup of any existing config
  3. Ask for confirmation before proceeding`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegister(app, args)
		},
	}

	cmd.Flags().BoolVar(&flagRegisterDryRun, "dry-run", false, "show what would happen without making changes")
	cmd.Flags().BoolVar(&flagRegisterForce, "force", false, "overwrite existing registration")
	cmd.Flags().Bool("all", false, "register with all supported integrations")

	cmd.AddCommand(newRegisterStatusCmd(app))
	cmd.AddCommand(newRegisterUnregisterCmd(app))
	cmd.AddCommand(newRegisterBackupsCmd(app))
	cmd.AddCommand(newRegisterRollbackCmd(app))

	return cmd
}

func newRegisterStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show registration status for all integrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegisterStatus(app)
		},
	}
}

func newRegisterUnregisterCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unregister [integration...]",
		Short: "Remove imggen from AI CLI tools",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnregister(app, args)
		},
	}
	cmd.Flags().BoolVar(&flagRegisterDryRun, "dry-run", false, "show what would happen without making changes")
	return cmd
}

func newRegisterBackupsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "backups [integration]",
		Short: "List backup files for an integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListBackups(app, args[0])
		},
	}
}

func newRegisterRollbackCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <backup-path>",
		Short: "Restore a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(app, args[0])
		},
	}
}

func runRegister(app *App, args []string) error {
	registrar := register.NewRegistrar(app.Out, app.Err, os.Stdin)
	registrar.DryRun = flagRegisterDryRun
	registrar.Force = flagRegisterForce

	var integrations []register.Integration

	// Check for --all flag
	allFlag := false
	for _, arg := range os.Args {
		if arg == "--all" {
			allFlag = true
			break
		}
	}

	if allFlag {
		integrations = register.AllIntegrations()
	} else if len(args) == 0 {
		// Show help if no integrations specified
		fmt.Fprintln(app.Out, "Available integrations:")
		for _, i := range register.AllIntegrations() {
			registered, _, _ := registrar.Status(i)
			status := "not registered"
			if registered {
				status = "registered"
			}
			fmt.Fprintf(app.Out, "  %-8s  %s (%s)\n", i, i.Description(), status)
		}
		fmt.Fprintln(app.Out, "\nUsage:")
		fmt.Fprintln(app.Out, "  imggen register --all           # Register with all integrations")
		fmt.Fprintln(app.Out, "  imggen register claude codex    # Register with specific integrations")
		fmt.Fprintln(app.Out, "  imggen register status          # Show detailed status")
		fmt.Fprintln(app.Out, "\nUse 'imggen register --help' for more options.")
		return nil
	} else {
		for _, arg := range args {
			i := register.Integration(arg)
			valid := false
			for _, all := range register.AllIntegrations() {
				if i == all {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("unknown integration %q: valid options are %v", arg, register.AllIntegrations())
			}
			integrations = append(integrations, i)
		}
	}

	if flagRegisterDryRun {
		fmt.Fprintln(app.Out, "DRY RUN - no changes will be made")
	}

	results := registrar.Register(integrations)

	// Summary
	fmt.Fprintln(app.Out, "\nSummary:")
	var succeeded, skipped, failed int
	for _, r := range results {
		if r.Error != nil {
			fmt.Fprintf(app.Out, "  ✗ %s: %v\n", r.Integration.DisplayName(), r.Error)
			failed++
		} else if r.WasSkipped {
			fmt.Fprintf(app.Out, "  - %s: %s\n", r.Integration.DisplayName(), r.SkipReason)
			skipped++
		} else {
			fmt.Fprintf(app.Out, "  ✓ %s: %s\n", r.Integration.DisplayName(), r.ConfigPath)
			succeeded++
		}
	}

	fmt.Fprintf(app.Out, "\n%d succeeded, %d skipped, %d failed\n", succeeded, skipped, failed)

	if failed > 0 {
		return fmt.Errorf("%d registration(s) failed", failed)
	}

	return nil
}

func runRegisterStatus(app *App) error {
	registrar := register.NewRegistrar(app.Out, app.Err, os.Stdin)

	fmt.Fprintln(app.Out, "Registration Status:")
	fmt.Fprintln(app.Out, "")

	for _, i := range register.AllIntegrations() {
		registered, configPath, err := registrar.Status(i)
		if err != nil {
			fmt.Fprintf(app.Out, "%-15s  Error: %v\n", i.DisplayName(), err)
			continue
		}

		status := "✗ Not registered"
		if registered {
			status = "✓ Registered"
		}

		fmt.Fprintf(app.Out, "%-15s  %s\n", i.DisplayName(), status)
		fmt.Fprintf(app.Out, "                 %s\n", configPath)

		// List backups
		backups, _ := registrar.ListBackups(i)
		if len(backups) > 0 {
			fmt.Fprintf(app.Out, "                 Backups: %d\n", len(backups))
		}
		fmt.Fprintln(app.Out, "")
	}

	return nil
}

func runUnregister(app *App, args []string) error {
	registrar := register.NewRegistrar(app.Out, app.Err, os.Stdin)
	registrar.DryRun = flagRegisterDryRun

	for _, arg := range args {
		i := register.Integration(arg)
		valid := false
		for _, all := range register.AllIntegrations() {
			if i == all {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown integration %q", arg)
		}

		if err := registrar.Unregister(i); err != nil {
			return err
		}
	}

	return nil
}

func runListBackups(app *App, integration string) error {
	registrar := register.NewRegistrar(app.Out, app.Err, os.Stdin)

	i := register.Integration(integration)
	valid := false
	for _, all := range register.AllIntegrations() {
		if i == all {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown integration %q", integration)
	}

	backups, err := registrar.ListBackups(i)
	if err != nil {
		return err
	}

	if len(backups) == 0 {
		fmt.Fprintf(app.Out, "No backups found for %s\n", i.DisplayName())
		return nil
	}

	fmt.Fprintf(app.Out, "Backups for %s:\n", i.DisplayName())
	for _, b := range backups {
		info, err := os.Stat(b)
		if err != nil {
			fmt.Fprintf(app.Out, "  %s\n", b)
		} else {
			fmt.Fprintf(app.Out, "  %s (%s)\n", b, info.ModTime().Format("2006-01-02 15:04:05"))
		}
	}

	fmt.Fprintln(app.Out, "\nTo restore a backup:")
	fmt.Fprintln(app.Out, "  imggen register rollback <backup-path>")

	return nil
}

func runRollback(app *App, backupPath string) error {
	registrar := register.NewRegistrar(app.Out, app.Err, os.Stdin)
	return registrar.Rollback(backupPath)
}

// OCR command

func newOCRCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ocr <image-path>",
		Short: "Extract text from images using OCR",
		Long: `Extract text from images using OpenAI's vision API.

Supports structured output with JSON schemas. If no schema is provided,
you can use --suggest-schema to have the AI suggest an appropriate schema.

Input:
  Provide an image file path as argument, or use --url for remote images.

Output:
  By default, outputs plain text. Use --schema for structured JSON output.

Examples:
  imggen ocr image.png                              # Extract text from image
  imggen ocr --url https://example.com/image.png    # Extract from URL
  imggen ocr image.png --schema schema.json         # Structured output
  imggen ocr image.png --suggest-schema             # Suggest a JSON schema
  imggen ocr image.png -o output.txt                # Save to file
  imggen ocr receipt.jpg --schema invoice.json -o data.json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if flagOCRURL != "" {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOCR(cmd, args, app)
		},
	}

	cmd.Flags().StringVarP(&flagOCRModel, "model", "m", "gpt-5-mini", "model to use (gpt-5.2, gpt-5-mini, gpt-5-nano)")
	cmd.Flags().StringVarP(&flagOCRSchema, "schema", "s", "", "JSON schema file for structured output")
	cmd.Flags().StringVar(&flagOCRSchemaName, "schema-name", "", "name for the JSON schema (default: extracted_data)")
	cmd.Flags().BoolVar(&flagOCRSuggestSchema, "suggest-schema", false, "suggest a JSON schema based on image content")
	cmd.Flags().StringVarP(&flagOCRPrompt, "prompt", "p", "", "custom extraction prompt")
	cmd.Flags().StringVarP(&flagOCROutput, "output", "o", "", "output file (default: stdout)")
	cmd.Flags().StringVar(&flagOCRURL, "url", "", "image URL instead of file path")
	cmd.Flags().StringVar(&flagAPIKey, "api-key", "", "API key (defaults to OPENAI_API_KEY)")
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log HTTP requests and responses")

	return cmd
}

func runOCR(_ *cobra.Command, args []string, app *App) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	apiKey, _, err := keys.GetAPIKey(flagAPIKey, "openai", "OPENAI_API_KEY")
	if err != nil {
		return err
	}

	providerCfg := &provider.Config{APIKey: apiKey, Verbose: flagVerbose}
	prov, err := app.NewProvider(providerCfg, app.Registry)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	ocrProv, ok := prov.(interface {
		OCR(ctx context.Context, req *models.OCRRequest) (*models.OCRResponse, error)
		SuggestSchema(ctx context.Context, req *models.OCRRequest) (json.RawMessage, error)
	})
	if !ok {
		return fmt.Errorf("provider does not support OCR")
	}

	req := models.NewOCRRequest()
	req.Model = flagOCRModel
	req.Prompt = flagOCRPrompt

	if flagOCRURL != "" {
		req.ImageURL = flagOCRURL
	} else if len(args) > 0 {
		req.ImagePath = args[0]
		if _, err := os.Stat(req.ImagePath); os.IsNotExist(err) {
			return fmt.Errorf("image file not found: %s", req.ImagePath)
		}
	}

	// Load schema if provided
	if flagOCRSchema != "" {
		schemaData, err := os.ReadFile(flagOCRSchema)
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}
		req.Schema = schemaData
		req.SchemaName = flagOCRSchemaName
	}

	// Suggest schema mode
	if flagOCRSuggestSchema {
		fmt.Fprintln(app.Out, "Analyzing image to suggest JSON schema...")
		schema, err := ocrProv.SuggestSchema(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to suggest schema: %w", err)
		}

		// Pretty print the schema
		var prettySchema bytes.Buffer
		if err := json.Indent(&prettySchema, schema, "", "  "); err != nil {
			fmt.Fprintln(app.Out, string(schema))
		} else {
			fmt.Fprintln(app.Out, prettySchema.String())
		}

		// Save to file if output specified
		if flagOCROutput != "" {
			if err := os.WriteFile(flagOCROutput, prettySchema.Bytes(), 0644); err != nil {
				return fmt.Errorf("failed to write schema file: %w", err)
			}
			fmt.Fprintf(app.Out, "\nSchema saved to: %s\n", flagOCROutput)
		}

		return nil
	}

	// Regular OCR extraction
	source := req.ImagePath
	if source == "" {
		source = req.ImageURL
	}
	fmt.Fprintf(app.Out, "Extracting text from %s using %s...\n", source, req.Model)

	resp, err := ocrProv.OCR(ctx, req)
	if err != nil {
		return fmt.Errorf("OCR failed: %w", err)
	}

	var output string
	if len(resp.Structured) > 0 {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, resp.Structured, "", "  "); err != nil {
			output = string(resp.Structured)
		} else {
			output = prettyJSON.String()
		}
	} else {
		output = resp.Text
	}

	// Write output
	if flagOCROutput != "" {
		if err := os.WriteFile(flagOCROutput, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(app.Out, "Output saved to: %s\n", flagOCROutput)
	} else {
		fmt.Fprintln(app.Out, "")
		fmt.Fprintln(app.Out, output)
	}

	// Show cost info
	if resp.Cost != nil {
		fmt.Fprintf(app.Out, "\nCost: $%.6f (input: %d tokens, output: %d tokens)\n",
			resp.Cost.Total, resp.InputTokens, resp.OutputTokens)

		// Log cost to database
		store, err := session.NewStore()
		if err == nil {
			defer store.Close()
			costEntry := &session.CostEntry{
				IterationID: "",
				SessionID:   "",
				Provider:    "openai",
				Model:       req.Model,
				Cost:        resp.Cost.Total,
				ImageCount:  1,
				Timestamp:   time.Now(),
			}
			if logErr := store.LogCost(ctx, costEntry); logErr != nil {
				fmt.Fprintf(app.Err, "Warning: failed to log cost: %v\n", logErr)
			}
		}
	}

	return nil
}

// Keys command

func newKeysCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys",
		Long: `Manage API keys for image generation providers.

Keys are stored securely in a local configuration file and are used
automatically when generating images. This is useful for CLI tools
that don't pass environment variables (like Codex CLI).

Key lookup order:
  1. --api-key flag (highest priority)
  2. Stored key in keys.json
  3. OPENAI_API_KEY environment variable

Examples:
  imggen keys set              # Save your OpenAI API key
  imggen keys                  # List stored keys
  imggen keys path             # Show keys.json location
  imggen keys delete           # Remove stored key`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysList(app)
		},
	}

	cmd.AddCommand(newKeysSetCmd(app))
	cmd.AddCommand(newKeysPathCmd(app))
	cmd.AddCommand(newKeysDeleteCmd(app))

	return cmd
}

func newKeysSetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "set",
		Short: "Save an API key",
		Long: `Save an API key for OpenAI.

The key will be stored in a local configuration file and used
automatically when generating images. This is an alternative to
setting the OPENAI_API_KEY environment variable.

Example:
  imggen keys set`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysSet(app)
		},
	}
}

func newKeysPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show the keys.json file location",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysPath(app)
		},
	}
}

func newKeysDeleteCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Delete a stored API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeysDelete(app)
		},
	}
}

func runKeysList(app *App) error {
	store, err := keys.NewStore()
	if err != nil {
		return err
	}

	providers, err := store.List()
	if err != nil {
		return err
	}

	if len(providers) == 0 {
		fmt.Fprintln(app.Out, "No API keys stored.")
		fmt.Fprintln(app.Out, "")
		fmt.Fprintln(app.Out, "To save a key:")
		fmt.Fprintln(app.Out, "  imggen keys set")
		return nil
	}

	fmt.Fprintln(app.Out, "Stored API keys:")
	fmt.Fprintln(app.Out, "")
	for _, provider := range providers {
		key, _ := store.Get(provider)
		fmt.Fprintf(app.Out, "  %-10s  %s\n", provider, keys.MaskKey(key))
	}
	fmt.Fprintln(app.Out, "")
	fmt.Fprintf(app.Out, "Keys file: %s\n", store.Path())

	return nil
}

func runKeysSet(app *App) error {
	store, err := keys.NewStore()
	if err != nil {
		return err
	}

	provider := "openai"
	envVar := "OPENAI_API_KEY"

	// Check if key already exists in store
	existing, _ := store.Get(provider)
	if existing != "" {
		fmt.Fprintf(app.Out, "Existing key for %s: %s\n", provider, keys.MaskKey(existing))
		fmt.Fprint(app.Out, "Replace it? [y/N] ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Fprintln(app.Out, "Cancelled.")
			return nil
		}
	}

	// Check if key exists in environment variable
	var key string
	if envKey := os.Getenv(envVar); envKey != "" {
		fmt.Fprintf(app.Out, "Found %s in environment: %s\n", envVar, keys.MaskKey(envKey))
		fmt.Fprint(app.Out, "Import this key? [Y/n] ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "" || response == "y" || response == "yes" {
			key = envKey
		}
	}

	// If not imported from env, prompt for key
	if key == "" {
		fmt.Fprintf(app.Out, "Enter API key for %s: ", provider)

		// Read key (hide input if terminal)
		if term.IsTerminal(int(os.Stdin.Fd())) {
			keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("failed to read key: %w", err)
			}
			key = string(keyBytes)
			fmt.Fprintln(app.Out, "") // newline after hidden input
		} else {
			reader := bufio.NewReader(os.Stdin)
			key, _ = reader.ReadString('\n')
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("no key provided")
		}
	}

	if err := store.Set(provider, key); err != nil {
		return err
	}

	fmt.Fprintf(app.Out, "Saved key for %s: %s\n", provider, keys.MaskKey(key))
	fmt.Fprintf(app.Out, "Keys file: %s\n", store.Path())

	return nil
}

func runKeysPath(app *App) error {
	store, err := keys.NewStore()
	if err != nil {
		return err
	}

	fmt.Fprintln(app.Out, store.Path())
	return nil
}

func runKeysDelete(app *App) error {
	store, err := keys.NewStore()
	if err != nil {
		return err
	}

	provider := "openai"

	// Check if key exists
	existing, _ := store.Get(provider)
	if existing == "" {
		fmt.Fprintf(app.Out, "No key stored for %s\n", provider)
		return nil
	}

	fmt.Fprintf(app.Out, "Delete key for %s? (%s) [y/N] ", provider, keys.MaskKey(existing))

	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Fprintln(app.Out, "Cancelled.")
		return nil
	}

	if err := store.Delete(provider); err != nil {
		return err
	}

	fmt.Fprintf(app.Out, "Deleted key for %s\n", provider)
	return nil
}
