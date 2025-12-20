package batch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestParseText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "basic prompts",
			input:   "prompt one\nprompt two\nprompt three",
			want:    3,
			wantErr: false,
		},
		{
			name:    "with empty lines",
			input:   "prompt one\n\nprompt two\n\n",
			want:    2,
			wantErr: false,
		},
		{
			name:    "with comments",
			input:   "# this is a comment\nprompt one\n# another comment\nprompt two",
			want:    2,
			wantErr: false,
		},
		{
			name:    "empty file",
			input:   "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "only comments",
			input:   "# comment\n# another",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := ParseText(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(items) != tt.want {
				t.Errorf("ParseText() got %d items, want %d", len(items), tt.want)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "basic array",
			input:   `[{"prompt": "one"}, {"prompt": "two"}]`,
			want:    2,
			wantErr: false,
		},
		{
			name:    "with options",
			input:   `[{"prompt": "one", "model": "dall-e-3", "quality": "hd"}]`,
			want:    1,
			wantErr: false,
		},
		{
			name:    "empty array",
			input:   `[]`,
			want:    0,
			wantErr: true,
		},
		{
			name:    "empty prompt",
			input:   `[{"prompt": ""}]`,
			want:    0,
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   `[{"prompt": "one"`,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := ParseJSON(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(items) != tt.want {
				t.Errorf("ParseJSON() got %d items, want %d", len(items), tt.want)
			}
		})
	}
}

func TestSanitizePrompt(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a sunset over mountains", "a-sunset-over-mountains"},
		{"Hello World!", "hello-world"},
		{"test@#$%^&*()prompt", "testprompt"},
		{"  multiple   spaces  ", "multiple-spaces"},
		{"", "image"},
		{strings.Repeat("a", 100), strings.Repeat("a", 50)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizePrompt(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePrompt(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateFilename(t *testing.T) {
	tests := []struct {
		index  int
		prompt string
		format models.OutputFormat
		want   string
	}{
		{1, "sunset mountains", models.FormatPNG, "001-sunset-mountains.png"},
		{10, "cat playing", models.FormatJPEG, "010-cat-playing.jpeg"},
		{100, "test", models.FormatWebP, "100-test.webp"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := generateFilename(tt.index, tt.prompt, tt.format)
			if got != tt.want {
				t.Errorf("generateFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
		Cost: &models.CostInfo{PerImage: 0.04, Total: 0.04},
	}, nil
}

func (m *mockProvider) Edit(ctx context.Context, req *models.EditRequest) (*models.Response, error) {
	return nil, nil
}

func (m *mockProvider) SupportsModel(model string) bool {
	return true
}

func (m *mockProvider) SupportsEdit(model string) bool {
	return model == "gpt-image-1" || model == "dall-e-2"
}

func (m *mockProvider) ListModels() []string {
	return []string{"gpt-image-1", "dall-e-3", "dall-e-2"}
}

func TestProcessorProcess(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	proc := NewProcessor(
		&mockProvider{},
		image.NewSaver(),
		models.DefaultRegistry(),
		out,
		errOut,
	)

	items := []Item{
		{Index: 1, Prompt: "test prompt one"},
		{Index: 2, Prompt: "test prompt two"},
	}

	opts := &Options{
		OutputDir:    t.TempDir(),
		DefaultModel: "gpt-image-1",
		Format:       models.FormatPNG,
		Parallel:     1,
	}

	results, err := proc.Process(context.Background(), items, opts)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Process() got %d results, want 2", len(results))
	}

	for i, r := range results {
		if r.Error != nil {
			t.Errorf("Result[%d] has error: %v", i, r.Error)
		}
		if r.Path == "" {
			t.Errorf("Result[%d] has empty path", i)
		}
	}
}

func TestProcessorWithConfig(t *testing.T) {
	tests := []struct {
		name    string
		items   []Item
		opts    *Options
		wantErr bool
	}{
		{
			name: "sequential processing",
			items: []Item{
				{Index: 1, Prompt: "prompt one"},
			},
			opts: &Options{
				OutputDir:    t.TempDir(),
				DefaultModel: "gpt-image-1",
				Format:       models.FormatPNG,
				Parallel:     1,
			},
			wantErr: false,
		},
		{
			name: "parallel processing",
			items: []Item{
				{Index: 1, Prompt: "prompt one"},
				{Index: 2, Prompt: "prompt two"},
			},
			opts: &Options{
				OutputDir:    t.TempDir(),
				DefaultModel: "gpt-image-1",
				Format:       models.FormatPNG,
				Parallel:     2,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			proc := NewProcessor(
				&mockProvider{},
				image.NewSaver(),
				models.DefaultRegistry(),
				out,
				out,
			)

			_, err := proc.Process(context.Background(), tt.items, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is a longer string", 10, "this is..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestPrintSummary(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		wantOut string
	}{
		{
			name: "all successful",
			results: []Result{
				{Index: 1, Prompt: "test one", Path: "/tmp/1.png", Cost: 0.04},
				{Index: 2, Prompt: "test two", Path: "/tmp/2.png", Cost: 0.04},
			},
			wantOut: "Successful: 2/2",
		},
		{
			name: "with failures",
			results: []Result{
				{Index: 1, Prompt: "test one", Path: "/tmp/1.png", Cost: 0.04},
				{Index: 2, Prompt: "test two", Error: fmt.Errorf("generation failed")},
			},
			wantOut: "Failed: 1",
		},
		{
			name:    "empty results",
			results: []Result{},
			wantOut: "Successful: 0/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			proc := NewProcessor(
				&mockProvider{},
				image.NewSaver(),
				models.DefaultRegistry(),
				out,
				out,
			)
			proc.PrintSummary(tt.results)
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("PrintSummary() output = %q, want to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		want     int
		wantErr  bool
	}{
		{
			name:     "txt file",
			filename: "test.txt",
			content:  "prompt one\nprompt two",
			want:     2,
			wantErr:  false,
		},
		{
			name:     "json file",
			filename: "test.json",
			content:  `[{"prompt": "one"}, {"prompt": "two"}]`,
			want:     2,
			wantErr:  false,
		},
		{
			name:     "unsupported extension",
			filename: "test.yaml",
			content:  "prompt: test",
			want:     0,
			wantErr:  true,
		},
		{
			name:     "no extension treated as txt",
			filename: "prompts",
			content:  "prompt one\nprompt two",
			want:     2,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := tmpDir + "/" + tt.filename
			if err := os.WriteFile(filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			items, err := ParseFile(filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(items) != tt.want {
				t.Errorf("ParseFile() got %d items, want %d", len(items), tt.want)
			}
		})
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/file.txt")
	if err == nil {
		t.Error("ParseFile() expected error for non-existent file")
	}
}

func TestProcessorWithErrors(t *testing.T) {
	t.Run("unknown model error", func(t *testing.T) {
		out := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		proc := NewProcessor(
			&mockProvider{},
			image.NewSaver(),
			models.DefaultRegistry(),
			out,
			errOut,
		)

		items := []Item{
			{Index: 1, Prompt: "test", Model: "unknown-model"},
		}

		opts := &Options{
			OutputDir:    t.TempDir(),
			DefaultModel: "gpt-image-1",
			Format:       models.FormatPNG,
			Parallel:     1,
		}

		results, _ := proc.Process(context.Background(), items, opts)
		if results[0].Error == nil {
			t.Error("Expected error for unknown model")
		}
	})

	t.Run("generation error", func(t *testing.T) {
		out := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		proc := NewProcessor(
			&mockProvider{
				generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
					return nil, fmt.Errorf("API error")
				},
			},
			image.NewSaver(),
			models.DefaultRegistry(),
			out,
			errOut,
		)

		items := []Item{
			{Index: 1, Prompt: "test"},
		}

		opts := &Options{
			OutputDir:    t.TempDir(),
			DefaultModel: "gpt-image-1",
			Format:       models.FormatPNG,
			Parallel:     1,
		}

		results, _ := proc.Process(context.Background(), items, opts)
		if results[0].Error == nil {
			t.Error("Expected error for generation failure")
		}
	})

	t.Run("stop on error", func(t *testing.T) {
		out := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		proc := NewProcessor(
			&mockProvider{
				generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
					return nil, fmt.Errorf("API error")
				},
			},
			image.NewSaver(),
			models.DefaultRegistry(),
			out,
			errOut,
		)

		items := []Item{
			{Index: 1, Prompt: "test one"},
			{Index: 2, Prompt: "test two"},
		}

		opts := &Options{
			OutputDir:    t.TempDir(),
			DefaultModel: "gpt-image-1",
			Format:       models.FormatPNG,
			Parallel:     1,
			StopOnError:  true,
		}

		_, err := proc.Process(context.Background(), items, opts)
		if err == nil {
			t.Error("Expected error when StopOnError is true")
		}
	})
}

func TestProcessorWithDelay(t *testing.T) {
	out := &bytes.Buffer{}
	proc := NewProcessor(
		&mockProvider{},
		image.NewSaver(),
		models.DefaultRegistry(),
		out,
		out,
	)

	items := []Item{
		{Index: 1, Prompt: "test one"},
		{Index: 2, Prompt: "test two"},
	}

	opts := &Options{
		OutputDir:    t.TempDir(),
		DefaultModel: "gpt-image-1",
		Format:       models.FormatPNG,
		Parallel:     1,
		DelayMs:      10,
	}

	_, err := proc.Process(context.Background(), items, opts)
	if err != nil {
		t.Errorf("Process() with delay error = %v", err)
	}
}

func TestProcessorContextCancellation(t *testing.T) {
	out := &bytes.Buffer{}
	proc := NewProcessor(
		&mockProvider{},
		image.NewSaver(),
		models.DefaultRegistry(),
		out,
		out,
	)

	items := []Item{
		{Index: 1, Prompt: "test one"},
		{Index: 2, Prompt: "test two"},
	}

	opts := &Options{
		OutputDir:    t.TempDir(),
		DefaultModel: "gpt-image-1",
		Format:       models.FormatPNG,
		Parallel:     1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := proc.Process(ctx, items, opts)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

func TestProcessorParallelWithMoreWorkersThanItems(t *testing.T) {
	out := &bytes.Buffer{}
	proc := NewProcessor(
		&mockProvider{},
		image.NewSaver(),
		models.DefaultRegistry(),
		out,
		out,
	)

	items := []Item{
		{Index: 1, Prompt: "test"},
	}

	opts := &Options{
		OutputDir:    t.TempDir(),
		DefaultModel: "gpt-image-1",
		Format:       models.FormatPNG,
		Parallel:     10, // More workers than items
	}

	results, err := proc.Process(context.Background(), items, opts)
	if err != nil {
		t.Errorf("Process() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Process() got %d results, want 1", len(results))
	}
}

func TestProcessItemWithCustomOptions(t *testing.T) {
	out := &bytes.Buffer{}
	proc := NewProcessor(
		&mockProvider{},
		image.NewSaver(),
		models.DefaultRegistry(),
		out,
		out,
	)

	items := []Item{
		{Index: 1, Prompt: "test", Size: "1024x1024", Quality: "high"},
	}

	opts := &Options{
		OutputDir:    t.TempDir(),
		DefaultModel: "gpt-image-1",
		Format:       models.FormatPNG,
		Parallel:     1,
	}

	results, err := proc.Process(context.Background(), items, opts)
	if err != nil {
		t.Errorf("Process() error = %v", err)
	}
	if results[0].Error != nil {
		t.Errorf("Process() result error = %v", results[0].Error)
	}
}

func TestProcessItemWithNoCost(t *testing.T) {
	out := &bytes.Buffer{}
	proc := NewProcessor(
		&mockProvider{
			generateFunc: func(ctx context.Context, req *models.Request) (*models.Response, error) {
				return &models.Response{
					Images: []models.GeneratedImage{
						{Data: []byte("test"), Index: 0},
					},
					Cost: nil, // No cost info
				}, nil
			},
		},
		image.NewSaver(),
		models.DefaultRegistry(),
		out,
		out,
	)

	items := []Item{
		{Index: 1, Prompt: "test"},
	}

	opts := &Options{
		OutputDir:    t.TempDir(),
		DefaultModel: "gpt-image-1",
		Format:       models.FormatPNG,
		Parallel:     1,
	}

	results, err := proc.Process(context.Background(), items, opts)
	if err != nil {
		t.Errorf("Process() error = %v", err)
	}
	if results[0].Cost != 0 {
		t.Errorf("Expected zero cost, got %v", results[0].Cost)
	}
}

var _ provider.Provider = (*mockProvider)(nil)
