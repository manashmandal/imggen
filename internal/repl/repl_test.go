package repl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		&CostCommand{},
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

// CostCommand Tests

func TestCostCommand_Metadata(t *testing.T) {
	cmd := &CostCommand{}

	if cmd.Name() != "cost" {
		t.Errorf("Name() = %s, want cost", cmd.Name())
	}

	aliases := cmd.Aliases()
	if len(aliases) != 1 || aliases[0] != "$" {
		t.Errorf("Aliases() = %v, want [$]", aliases)
	}

	if cmd.Description() == "" {
		t.Error("Description() returned empty string")
	}

	if !strings.Contains(cmd.Usage(), "cost") {
		t.Errorf("Usage() = %s, should contain 'cost'", cmd.Usage())
	}
}

func TestCostCommand_Registered(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "")
	defer cleanup()

	if _, ok := r.commands["cost"]; !ok {
		t.Error("cost command not registered")
	}
	if _, ok := r.commands["$"]; !ok {
		t.Error("$ alias not registered")
	}
}

func TestCostCommand_Total_NoCosts(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded") {
		t.Error("cost command did not show 'No costs recorded' message")
	}
}

func TestCostCommand_Total_Subcommand_NoCosts(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost total\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded") {
		t.Error("cost total did not show 'No costs recorded' message")
	}
}

func TestCostCommand_Today_NoCosts(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost today\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded today") {
		t.Error("cost today did not show appropriate message")
	}
}

func TestCostCommand_Week_NoCosts(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost week\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded in the last 7 days") {
		t.Error("cost week did not show appropriate message")
	}
}

func TestCostCommand_Month_NoCosts(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost month\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded in the last 30 days") {
		t.Error("cost month did not show appropriate message")
	}
}

func TestCostCommand_Provider_NoCosts(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost provider\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded") {
		t.Error("cost provider did not show 'No costs recorded' message")
	}
}

func TestCostCommand_Session_NoSession(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "cost session\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No active session") {
		t.Error("cost session did not show 'No active session' message")
	}
}

func TestCostCommand_Session_NoCostsInSession(t *testing.T) {
	r, out, mgr, cleanup := testREPL(t, "session new test\ncost session\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !mgr.HasSession() {
		t.Error("session should exist")
	}

	if !strings.Contains(out.String(), "No costs in current session") {
		t.Error("cost session did not show 'No costs in current session' message")
	}
}

func TestCostCommand_UnknownSubcommand(t *testing.T) {
	r, _, _, cleanup := testREPL(t, "cost unknown\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestCostCommand_DollarAlias(t *testing.T) {
	r, out, _, cleanup := testREPL(t, "$\nquit\n")
	defer cleanup()

	ctx := context.Background()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(out.String(), "No costs recorded") {
		t.Error("$ alias did not work for cost command")
	}
}

func testREPLWithCosts(t *testing.T, input string) (*REPL, *bytes.Buffer, *session.Manager, func()) {
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

func createIterationWithCost(t *testing.T, ctx context.Context, mgr *session.Manager, cost float64, imageCount int) {
	t.Helper()

	iter := &session.Iteration{
		Operation: "generate",
		Prompt:    "test prompt",
		Model:     "gpt-image-1",
		ImagePath: "/tmp/test.png",
		Metadata: session.IterationMetadata{
			Size:     "1024x1024",
			Quality:  "medium",
			Cost:     cost,
			Provider: "openai",
		},
	}
	if err := mgr.AddIteration(ctx, iter); err != nil {
		t.Fatalf("AddIteration() error = %v", err)
	}

	entry := &session.CostEntry{
		IterationID: iter.ID,
		SessionID:   mgr.Current().ID,
		Provider:    "openai",
		Model:       "gpt-image-1",
		Cost:        cost,
		ImageCount:  imageCount,
		Timestamp:   time.Now(),
	}
	if err := mgr.LogCost(ctx, entry); err != nil {
		t.Fatalf("LogCost() error = %v", err)
	}
}

func TestCostCommand_WithCosts(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	// Create a session
	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.042, 1)

	// Test total command
	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"total"}); err != nil {
		t.Errorf("Execute(total) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Total cost:") {
		t.Error("cost total did not show cost summary")
	}
	if !strings.Contains(output, "$0.0420") {
		t.Errorf("cost total did not show correct amount, got: %s", output)
	}
}

func TestCostCommand_Today_WithCosts(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.167, 1)

	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"today"}); err != nil {
		t.Errorf("Execute(today) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Today's cost:") {
		t.Error("cost today did not show cost summary")
	}
	if !strings.Contains(output, "$0.1670") {
		t.Errorf("cost today did not show correct amount, got: %s", output)
	}
}

func TestCostCommand_Provider_WithCosts(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	// Log multiple costs
	createIterationWithCost(t, ctx, mgr, 0.042, 1)
	createIterationWithCost(t, ctx, mgr, 0.080, 1)

	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"provider"}); err != nil {
		t.Errorf("Execute(provider) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Provider") {
		t.Error("cost provider did not show header")
	}
	if !strings.Contains(output, "openai") {
		t.Error("cost provider did not show openai provider")
	}
	if !strings.Contains(output, "Total") {
		t.Error("cost provider did not show total row")
	}
}

func TestCostCommand_Session_WithCosts(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.011, 1)

	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"session"}); err != nil {
		t.Errorf("Execute(session) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Session cost:") {
		t.Error("cost session did not show session cost")
	}
	if !strings.Contains(output, "$0.0110") {
		t.Errorf("cost session did not show correct amount, got: %s", output)
	}
}

func TestCostCommand_Week_WithCosts(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.250, 1)

	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"week"}); err != nil {
		t.Errorf("Execute(week) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Last 7 days cost:") {
		t.Error("cost week did not show weekly cost")
	}
	if !strings.Contains(output, "$0.2500") {
		t.Errorf("cost week did not show correct amount, got: %s", output)
	}
}

func TestCostCommand_Month_WithCosts(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.063, 1)

	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"month"}); err != nil {
		t.Errorf("Execute(month) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Last 30 days cost:") {
		t.Error("cost month did not show monthly cost")
	}
	if !strings.Contains(output, "$0.0630") {
		t.Errorf("cost month did not show correct amount, got: %s", output)
	}
}

func TestCostCommand_MultipleImages(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.126, 3) // 3 images at $0.042 each

	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{"total"}); err != nil {
		t.Errorf("Execute(total) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "3 image(s)") {
		t.Errorf("cost total did not show correct image count, got: %s", output)
	}
}

func TestCostCommand_DefaultWithoutArgs(t *testing.T) {
	r, out, mgr, cleanup := testREPLWithCosts(t, "")
	defer cleanup()

	ctx := context.Background()

	if _, err := mgr.StartNew(ctx, "test-session"); err != nil {
		t.Fatalf("StartNew() error = %v", err)
	}

	createIterationWithCost(t, ctx, mgr, 0.042, 1)

	// Execute without subcommand - should default to total
	cmd := &CostCommand{}
	if err := cmd.Execute(ctx, r, []string{}); err != nil {
		t.Errorf("Execute() without args error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Total cost:") {
		t.Error("cost without args did not default to total")
	}
}
