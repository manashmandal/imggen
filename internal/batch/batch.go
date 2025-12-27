package batch

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

type Result struct {
	Index    int
	Prompt   string
	Path     string
	Cost     float64
	Error    error
	Duration time.Duration
}

type Options struct {
	OutputDir      string
	DefaultModel   string
	DefaultSize    string
	DefaultQuality string
	Format         models.OutputFormat
	Parallel       int
	StopOnError    bool
	DelayMs        int
}

type Processor struct {
	provider provider.Provider
	saver    *image.Saver
	registry *models.ModelRegistry
	out      io.Writer
	err      io.Writer
	outMu    sync.Mutex
}

func NewProcessor(prov provider.Provider, saver *image.Saver, registry *models.ModelRegistry, out, errOut io.Writer) *Processor {
	return &Processor{
		provider: prov,
		saver:    saver,
		registry: registry,
		out:      out,
		err:      errOut,
	}
}

func (p *Processor) printf(format string, args ...interface{}) {
	p.outMu.Lock()
	fmt.Fprintf(p.out, format, args...)
	p.outMu.Unlock()
}

func (p *Processor) errorf(format string, args ...interface{}) {
	p.outMu.Lock()
	fmt.Fprintf(p.err, format, args...)
	p.outMu.Unlock()
}

func (p *Processor) Process(ctx context.Context, items []Item, opts *Options) ([]Result, error) {
	if opts.Parallel <= 1 {
		return p.processSequential(ctx, items, opts)
	}
	return p.processParallel(ctx, items, opts)
}

func (p *Processor) processSequential(ctx context.Context, items []Item, opts *Options) ([]Result, error) {
	results := make([]Result, len(items))
	total := len(items)

	for i, item := range items {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result := p.processItem(ctx, item, opts, i+1, total)
		results[i] = result

		if result.Error != nil && opts.StopOnError {
			return results, fmt.Errorf("stopped at item %d: %w", i+1, result.Error)
		}

		if opts.DelayMs > 0 && i < len(items)-1 {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			case <-time.After(time.Duration(opts.DelayMs) * time.Millisecond):
			}
		}
	}

	return results, nil
}

func (p *Processor) processParallel(ctx context.Context, items []Item, opts *Options) ([]Result, error) {
	results := make([]Result, len(items))
	total := len(items)

	type job struct {
		index int
		item  Item
	}

	jobs := make(chan job, len(items))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	workers := opts.Parallel
	if workers > len(items) {
		workers = len(items)
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				result := p.processItem(ctx, j.item, opts, j.index+1, total)

				mu.Lock()
				results[j.index] = result
				if result.Error != nil && opts.StopOnError && firstErr == nil {
					firstErr = result.Error
				}
				mu.Unlock()

				if opts.StopOnError && firstErr != nil {
					return
				}
			}
		}()
	}

	for i, item := range items {
		if opts.StopOnError && firstErr != nil {
			break
		}
		jobs <- job{index: i, item: item}
	}
	close(jobs)

	wg.Wait()

	if firstErr != nil {
		return results, fmt.Errorf("batch stopped due to error: %w", firstErr)
	}

	return results, nil
}

func (p *Processor) processItem(ctx context.Context, item Item, opts *Options, current, total int) Result {
	start := time.Now()
	result := Result{
		Index:  item.Index,
		Prompt: item.Prompt,
	}

	promptDisplay := truncate(item.Prompt, 50)
	p.printf("[%d/%d] Generating: %q...\n", current, total, promptDisplay)

	model := item.Model
	if model == "" {
		model = opts.DefaultModel
	}

	req := models.NewRequest(item.Prompt)
	req.Model = model
	req.Format = opts.Format

	if item.Size != "" {
		req.Size = item.Size
	} else if opts.DefaultSize != "" {
		req.Size = opts.DefaultSize
	}

	if item.Quality != "" {
		req.Quality = item.Quality
	} else if opts.DefaultQuality != "" {
		req.Quality = opts.DefaultQuality
	}

	if item.Style != "" {
		req.Style = item.Style
	}

	caps, ok := p.registry.Get(model)
	if !ok {
		result.Error = fmt.Errorf("unknown model: %s", model)
		result.Duration = time.Since(start)
		p.errorf("       Error: %v\n", result.Error)
		return result
	}
	caps.ApplyDefaults(req)

	if err := caps.Validate(req); err != nil {
		result.Error = fmt.Errorf("validation failed: %w", err)
		result.Duration = time.Since(start)
		p.errorf("       Error: %v\n", result.Error)
		return result
	}

	resp, err := p.provider.Generate(ctx, req)
	if err != nil {
		result.Error = fmt.Errorf("generation failed: %w", err)
		result.Duration = time.Since(start)
		p.errorf("       Error: %v\n", result.Error)
		return result
	}

	filename := generateFilename(item.Index, item.Prompt, opts.Format)
	outputPath := filepath.Join(opts.OutputDir, filename)

	paths, err := p.saver.SaveAll(ctx, resp, outputPath, opts.Format)
	if err != nil {
		result.Error = fmt.Errorf("save failed: %w", err)
		result.Duration = time.Since(start)
		p.errorf("       Error: %v\n", result.Error)
		return result
	}

	result.Path = paths[0]
	result.Duration = time.Since(start)

	if resp.Cost != nil {
		result.Cost = resp.Cost.Total
		p.printf("       Saved: %s ($%.4f)\n", result.Path, result.Cost)
	} else {
		p.printf("       Saved: %s\n", result.Path)
	}

	return result
}

func generateFilename(index int, prompt string, format models.OutputFormat) string {
	sanitized := sanitizePrompt(prompt)
	return fmt.Sprintf("%03d-%s.%s", index, sanitized, format)
}

var windowsReservedNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true,
	"com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true,
	"lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

func sanitizePrompt(prompt string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9\s-]`)
	sanitized := reg.ReplaceAllString(prompt, "")
	sanitized = strings.ToLower(sanitized)
	sanitized = strings.Join(strings.Fields(sanitized), "-")
	sanitized = strings.TrimLeft(sanitized, "-")

	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	sanitized = strings.TrimSuffix(sanitized, "-")

	if sanitized == "" {
		sanitized = "image"
	}

	if windowsReservedNames[sanitized] {
		sanitized = sanitized + "-img"
	}

	return sanitized
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (p *Processor) PrintSummary(results []Result) {
	var successful, failed int
	var totalCost float64
	var errors []Result

	for _, r := range results {
		if r.Error != nil {
			failed++
			errors = append(errors, r)
		} else {
			successful++
			totalCost += r.Cost
		}
	}

	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, "Summary:")
	fmt.Fprintf(p.out, "  Successful: %d/%d images\n", successful, len(results))
	if failed > 0 {
		fmt.Fprintf(p.out, "  Failed: %d (see errors below)\n", failed)
	}
	fmt.Fprintf(p.out, "  Total cost: $%.4f\n", totalCost)

	if len(errors) > 0 {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out, "Errors:")
		for _, e := range errors {
			promptDisplay := truncate(e.Prompt, 40)
			fmt.Fprintf(p.out, "  [%d] %q: %v\n", e.Index, promptDisplay, e.Error)
		}
	}
}
