package workflow

import (
	"os"
	"path/filepath"
	"testing"
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

func writeTempWorkflow(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTempWorkflow() error = %v", err)
	}
	return path
}
