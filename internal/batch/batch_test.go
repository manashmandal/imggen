package batch

import (
	"bytes"
	"context"
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

var _ provider.Provider = (*mockProvider)(nil)
