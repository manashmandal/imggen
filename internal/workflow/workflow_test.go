package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

func TestParseFile_Wrapper(t *testing.T) {
	content := `
workflow:
  name: "test"
  params:
    ref: "./ref.png"
  steps:
    - id: one
      prompt: "hello"
`
	path := writeTempWorkflow(t, content)

	wf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if wf.Name != "test" {
		t.Errorf("workflow name = %q, want %q", wf.Name, "test")
	}
	if len(wf.Steps) != 1 {
		t.Fatalf("workflow steps = %d, want 1", len(wf.Steps))
	}
}

func TestParseFile_Direct(t *testing.T) {
	content := `
name: "test"
steps:
  - id: one
    prompt: "hello"
`
	path := writeTempWorkflow(t, content)

	wf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if wf.Name != "test" {
		t.Errorf("workflow name = %q, want %q", wf.Name, "test")
	}
}

func TestWorkflowValidate_DependsOnUnknown(t *testing.T) {
	wf := &Workflow{
		Steps: []Step{
			{ID: "one", Prompt: "hello", DependsOn: []string{"missing"}},
		},
	}
	if err := wf.Validate(); err == nil {
		t.Fatal("Validate() expected error for unknown dependency")
	}
}

func TestApplyParams(t *testing.T) {
	params := map[string]string{"name": "hero"}
	out, err := applyParams("hello ${name}", params)
	if err != nil {
		t.Fatalf("applyParams() error = %v", err)
	}
	if out != "hello hero" {
		t.Fatalf("applyParams() = %q, want %q", out, "hello hero")
	}
}

func TestApplyParamsMissing(t *testing.T) {
	_, err := applyParams("hello ${missing}", map[string]string{})
	if err == nil {
		t.Fatal("applyParams() expected error for missing param")
	}
}

func TestTopoOrderCycle(t *testing.T) {
	steps := []Step{
		{ID: "a", Prompt: "one", DependsOn: []string{"b"}},
		{ID: "b", Prompt: "two", DependsOn: []string{"a"}},
	}
	if _, err := topoOrder(steps); err == nil {
		t.Fatal("topoOrder() expected error for cycle")
	}
}

func TestEngineRunOutputs(t *testing.T) {
	outDir := t.TempDir()
	registry := models.DefaultRegistry()

	prov := &mockProvider{
		generateFunc: func(_ context.Context, req *models.Request) (*models.Response, error) {
			if req.Prompt == "" {
				t.Fatal("expected prompt")
			}
			return &models.Response{
				Images: []models.GeneratedImage{
					{Data: []byte("img1"), Index: 0},
					{Data: []byte("img2"), Index: 1},
				},
				Cost: &models.CostInfo{Total: 0.08},
			}, nil
		},
	}

	engine := NewEngine(prov, image.NewSaver(), registry, os.Stdout, os.Stderr)

	wf := &Workflow{
		Params: map[string]string{"ref": "base.png"},
		Steps: []Step{
			{
				ID:     "one",
				Prompt: "hello ${ref}",
				Outputs: OutputSpec{
					Pattern: "out_{i}.png",
				},
			},
		},
	}

	results, err := engine.Run(context.Background(), wf, &RunOptions{
		OutputDir:     outDir,
		DefaultModel:  "gpt-image-1",
		DefaultFormat: models.FormatPNG,
		Params:        map[string]string{},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(results) != 1 || len(results[0].Paths) != 2 {
		t.Fatalf("unexpected results: %+v", results)
	}

	for _, path := range results[0].Paths {
		if !strings.Contains(filepath.Base(path), "out_") {
			t.Fatalf("expected output pattern, got %s", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
	}
}

func writeTempWorkflow(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTempWorkflow() error = %v", err)
	}
	return path
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
		Images: []models.GeneratedImage{{Data: []byte("test"), Index: 0}},
	}, nil
}

func (m *mockProvider) Edit(_ context.Context, _ *models.EditRequest) (*models.Response, error) {
	return nil, provider.ErrEditNotSupported
}

func (m *mockProvider) SupportsModel(_ string) bool {
	return true
}

func (m *mockProvider) SupportsEdit(_ string) bool {
	return true
}

func (m *mockProvider) ListModels() []string {
	return []string{"gpt-image-1"}
}
