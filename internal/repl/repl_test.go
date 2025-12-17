package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manash/imggen/internal/display"
	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/internal/session"
	"github.com/manash/imggen/pkg/models"
)

type mockProvider struct {
	generateFunc func(ctx context.Context, req *models.Request) (*models.Response, error)
	editFunc     func(ctx context.Context, req *models.EditRequest) (*models.Response, error)
	supportsEdit bool
}

func (m *mockProvider) Name() models.ProviderType {
	return models.ProviderOpenAI
}

func (m *mockProvider) Generate(ctx context.Context, req *models.Request) (*models.Response, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return &models.Response{
		Images: []models.GeneratedImage{{Data: []byte("test")}},
	}, nil
}

func (m *mockProvider) Edit(ctx context.Context, req *models.EditRequest) (*models.Response, error) {
	if m.editFunc != nil {
		return m.editFunc(ctx, req)
	}
	return nil, provider.ErrEditNotSupported
}

func (m *mockProvider) SupportsModel(_ string) bool {
	return true
}

func (m *mockProvider) SupportsEdit(_ string) bool {
	return m.supportsEdit
}

func (m *mockProvider) ListModels() []string {
	return []string{"gpt-image-1"}
}

func testREPL(t *testing.T, input string) (*REPL, *bytes.Buffer, *session.Manager, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)

	store, err := session.NewStoreWithPath(dbPath)
	if err != nil {
		t.Fatalf("NewStoreWithPath() error = %v", err)
	}

	mgr := session.NewManager(store, "gpt-image-1")
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	cfg := &Config{
		In:         strings.NewReader(input),
		Out:        out,
		Err:        errBuf,
		Provider:   &mockProvider{},
		Registry:   models.DefaultRegistry(),
		SessionMgr: mgr,
		Displayer:  display.New(out),
		Saver:      image.NewSaver(),
	}

	r := New(cfg)

	cleanup := func() {
		store.Close()
		os.Setenv("HOME", origHome)
	}

	return r, out, mgr, cleanup
}

func TestNew(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "")
	defer cleanup()

	if r == nil {
		t.Error("New() returned nil")
	}
	if len(r.commands) == 0 {
		t.Error("New() commands not registered")
	}
}

func TestREPL_CommandsRegistered(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "")
	defer cleanup()

	expectedCommands := []string{
		"generate", "gen", "g",
		"edit", "e",
		"undo", "u", "back",
		"save", "s",
		"show", "display", "view",
		"history", "h", "hist",
		"session", "sess",
		"model", "m",
		"help", "?",
		"quit", "exit", "q",
	}

	for _, cmd := range expectedCommands {
		if _, ok := r.commands[cmd]; !ok {
			t.Errorf("Command %q not registered", cmd)
		}
	}
}

func TestREPL_Run_Quit(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "quit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "Goodbye!") {
		t.Error("Run() quit command did not output 'Goodbye!'")
	}
}

func TestREPL_Run_Help(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "help\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Available commands") {
		t.Error("Run() help did not show available commands")
	}
	if !strings.Contains(output, "generate") {
		t.Error("Run() help did not list generate command")
	}
}

func TestREPL_Run_UnknownCommand(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "unknowncommand\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestREPL_Run_EmptyLine(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "\n\n\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestREPL_Stop(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "")
	defer cleanup()

	r.running = true
	r.Stop()

	if r.running {
		t.Error("Stop() did not stop the REPL")
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple command",
			input: "generate hello",
			want:  []string{"generate", "hello"},
		},
		{
			name:  "double quotes",
			input: `generate "hello world"`,
			want:  []string{"generate", "hello world"},
		},
		{
			name:  "single quotes",
			input: `generate 'hello world'`,
			want:  []string{"generate", "hello world"},
		},
		{
			name:  "multiple arguments",
			input: "session load abc123",
			want:  []string{"session", "load", "abc123"},
		},
		{
			name:  "mixed quotes",
			input: `model "gpt-image-1"`,
			want:  []string{"model", "gpt-image-1"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace only",
			input: "   ",
			want:  nil,
		},
		{
			name:  "multiple spaces",
			input: "generate    test    prompt",
			want:  []string{"generate", "test", "prompt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommand(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommand() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommand()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "needs truncation",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModelCommand_GetModel(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "model\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "gpt-image-1") {
		t.Error("model command did not show current model")
	}
	if !strings.Contains(output, "Available models") {
		t.Error("model command did not show available models")
	}
}

func TestModelCommand_SetModel(t *testing.T) {
	r, out, mgr, cleanup := testREPL(t, "model dall-e-3\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if mgr.GetModel() != "dall-e-3" {
		t.Errorf("model not changed, got %s, want dall-e-3", mgr.GetModel())
	}

	if !strings.Contains(out.String(), "Model set to: dall-e-3") {
		t.Error("model command did not confirm model change")
	}
}

func TestModelCommand_UnknownModel(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "model nonexistent\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestHistoryCommand_Empty(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "history\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No history") {
		t.Error("history command did not show empty message")
	}
}

func TestSessionCommand_List_Empty(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "session list\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No sessions") {
		t.Error("session list did not show empty message")
	}
}

func TestSessionCommand_New(t *testing.T) {
	r, out, mgr, cleanup := testREPL(t, "session new Test Session\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !mgr.HasSession() {
		t.Error("session new did not create session")
	}

	if !strings.Contains(out.String(), "Created new session") {
		t.Error("session new did not confirm creation")
	}
}

func TestGenerateCommand_NoPrompt(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "generate\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestEditCommand_NoIteration(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "edit add something\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestUndoCommand_NoIteration(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "undo\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestShowCommand_NoIteration(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "show\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestSaveCommand_NoIteration(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "save\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestCommand_Interface(t *testing.T) {
	commands := []Command{
		&GenerateCommand{},
		&EditCommand{},
		&UndoCommand{},
		&SaveCommand{},
		&ShowCommand{},
		&HistoryCommand{},
		&SessionCommand{},
		&ModelCommand{},
		&HelpCommand{},
		&QuitCommand{},
	}

	for _, cmd := range commands {
		t.Run(cmd.Name(), func(t *testing.T) {
			if cmd.Name() == "" {
				t.Error("Name() returned empty string")
			}
			if cmd.Description() == "" {
				t.Error("Description() returned empty string")
			}
			if cmd.Usage() == "" {
				t.Error("Usage() returned empty string")
			}
			// Aliases can be nil, that's ok
		})
	}
}
