package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/manash/imggen/internal/cost"
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
	flagVerbose     bool
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
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "log HTTP requests and responses (API keys redacted)")

	cmd.AddCommand(newCostCmd(app))
	cmd.AddCommand(newDBCmd(app))
	cmd.AddCommand(newPriceCmd(app))

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

func newPriceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "price",
		Short: "Manage pricing data",
		Long: `Manage pricing data used for cost calculations.

Custom prices override built-in defaults and are stored in ~/.imggen/pricing.json`,
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current pricing",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPriceShow(app)
		},
	}

	setCmd := &cobra.Command{
		Use:   "set <model> <size> <quality> <price>",
		Short: "Set a custom price",
		Long: `Set a custom price for a specific model/size/quality combination.

Examples:
  imggen price set gpt-image-1 1024x1024 low 0.011
  imggen price set dall-e-3 1024x1024 standard 0.040
  imggen price set dall-e-2 1024x1024 "" 0.020  # no quality for dall-e-2`,
		Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPriceSet(app, args)
		},
	}

	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset to built-in pricing",
		Long:  `Remove custom pricing and revert to built-in defaults.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPriceReset(app)
		},
	}

	cmd.AddCommand(showCmd)
	cmd.AddCommand(setCmd)
	cmd.AddCommand(resetCmd)

	return cmd
}

func runPriceSet(app *App, args []string) error {
	model := args[0]
	size := args[1]
	quality := args[2]

	var price float64
	if _, err := fmt.Sscanf(args[3], "%f", &price); err != nil {
		return fmt.Errorf("invalid price %q: must be a number", args[3])
	}

	if price <= 0 {
		return fmt.Errorf("price must be positive")
	}

	if err := cost.SetPrice(model, size, quality, price); err != nil {
		return fmt.Errorf("failed to set price: %w", err)
	}

	if quality != "" {
		fmt.Fprintf(app.Out, "Price set: %s %s %s = $%.4f\n", model, size, quality, price)
	} else {
		fmt.Fprintf(app.Out, "Price set: %s %s = $%.4f\n", model, size, price)
	}
	return nil
}

func runPriceReset(app *App) error {
	if err := cost.DeletePricing(); err != nil {
		return fmt.Errorf("failed to reset pricing: %w", err)
	}
	fmt.Fprintln(app.Out, "Custom pricing removed. Using built-in defaults.")
	return nil
}

func runPriceShow(app *App) error {
	pricing, err := cost.LoadPricing()
	if err != nil {
		return fmt.Errorf("failed to load pricing: %w", err)
	}

	if pricing == nil {
		fmt.Fprintln(app.Out, "Using built-in defaults (no custom pricing set)")
		fmt.Fprintln(app.Out)
		showBuiltinPricing(app)
		fmt.Fprintln(app.Out)
		fmt.Fprintln(app.Out, "To customize prices, use: imggen price set <model> <size> <quality> <price>")
		return nil
	}

	cachePath, _ := cost.PricingCachePath()
	fmt.Fprintf(app.Out, "Custom pricing (updated: %s)\n", pricing.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(app.Out, "Config: %s\n\n", cachePath)

	for model, prices := range pricing.Image {
		fmt.Fprintf(app.Out, "%s:\n", model)
		for key, price := range prices {
			quality, size := cost.ParsePricingKey(key)
			if quality != "" {
				fmt.Fprintf(app.Out, "  %s %s: $%.4f\n", size, quality, price)
			} else {
				fmt.Fprintf(app.Out, "  %s: $%.4f\n", size, price)
			}
		}
		fmt.Fprintln(app.Out)
	}

	fmt.Fprintln(app.Out, "Note: Custom prices override built-in defaults for matching configurations.")
	fmt.Fprintln(app.Out, "Run 'imggen price reset' to remove custom pricing.")

	return nil
}

func showBuiltinPricing(app *App) {
	fmt.Fprintln(app.Out, "Built-in pricing (from https://openai.com/api/pricing):")
	fmt.Fprintln(app.Out)

	fmt.Fprintln(app.Out, "gpt-image-1:")
	fmt.Fprintln(app.Out, "  1024x1024 low: $0.0110")
	fmt.Fprintln(app.Out, "  1024x1024 medium: $0.0420")
	fmt.Fprintln(app.Out, "  1024x1024 high: $0.1670")
	fmt.Fprintln(app.Out, "  1536x1024 low: $0.0160")
	fmt.Fprintln(app.Out, "  1536x1024 medium: $0.0630")
	fmt.Fprintln(app.Out, "  1536x1024 high: $0.2500")
	fmt.Fprintln(app.Out)

	fmt.Fprintln(app.Out, "dall-e-3:")
	fmt.Fprintln(app.Out, "  1024x1024 standard: $0.0400")
	fmt.Fprintln(app.Out, "  1024x1024 hd: $0.0800")
	fmt.Fprintln(app.Out, "  1792x1024 standard: $0.0800")
	fmt.Fprintln(app.Out, "  1792x1024 hd: $0.1200")
	fmt.Fprintln(app.Out)

	fmt.Fprintln(app.Out, "dall-e-2:")
	fmt.Fprintln(app.Out, "  1024x1024: $0.0200")
	fmt.Fprintln(app.Out, "  512x512: $0.0180")
	fmt.Fprintln(app.Out, "  256x256: $0.0160")
}
