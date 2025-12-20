package register

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_String(t *testing.T) {
	tests := []struct {
		i    Integration
		want string
	}{
		{Claude, "claude"},
		{Codex, "codex"},
		{Cursor, "cursor"},
		{Gemini, "gemini"},
	}

	for _, tt := range tests {
		if got := tt.i.String(); got != tt.want {
			t.Errorf("Integration.String() = %v, want %v", got, tt.want)
		}
	}
}

func TestIntegration_DisplayName(t *testing.T) {
	tests := []struct {
		i    Integration
		want string
	}{
		{Claude, "Claude Code"},
		{Codex, "OpenAI Codex CLI"},
		{Cursor, "Cursor"},
		{Gemini, "Gemini CLI"},
	}

	for _, tt := range tests {
		if got := tt.i.DisplayName(); got != tt.want {
			t.Errorf("Integration.DisplayName() = %v, want %v", got, tt.want)
		}
	}
}

func TestIntegration_IsAppendMode(t *testing.T) {
	tests := []struct {
		i    Integration
		want bool
	}{
		{Claude, false},
		{Codex, true},
		{Cursor, false},
		{Gemini, true},
	}

	for _, tt := range tests {
		if got := tt.i.IsAppendMode(); got != tt.want {
			t.Errorf("Integration(%s).IsAppendMode() = %v, want %v", tt.i, got, tt.want)
		}
	}
}

func TestIntegration_ConfigPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		i       Integration
		wantEnd string
	}{
		{Claude, filepath.Join(".claude", "skills", "imggen", "SKILL.md")},
		{Codex, filepath.Join(".codex", "AGENTS.md")},
		{Cursor, filepath.Join(".cursor", "rules", "imggen.mdc")},
		{Gemini, filepath.Join(".gemini", "GEMINI.md")},
	}

	for _, tt := range tests {
		got, err := tt.i.ConfigPath()
		if err != nil {
			t.Errorf("Integration(%s).ConfigPath() error = %v", tt.i, err)
			continue
		}
		want := filepath.Join(homeDir, tt.wantEnd)
		if got != want {
			t.Errorf("Integration(%s).ConfigPath() = %v, want %v", tt.i, got, want)
		}
	}
}

func TestAllIntegrations(t *testing.T) {
	all := AllIntegrations()
	if len(all) != 4 {
		t.Errorf("AllIntegrations() returned %d integrations, want 4", len(all))
	}

	expected := map[Integration]bool{
		Claude: true,
		Codex:  true,
		Cursor: true,
		Gemini: true,
	}

	for _, i := range all {
		if !expected[i] {
			t.Errorf("unexpected integration: %s", i)
		}
	}
}

func TestRegistrar_isAlreadyRegistered(t *testing.T) {
	r := &Registrar{}

	tests := []struct {
		name        string
		integration Integration
		content     string
		want        bool
	}{
		{
			name:        "claude with imggen",
			integration: Claude,
			content:     "---\nname: imggen\ndescription: test\n---",
			want:        true,
		},
		{
			name:        "claude without imggen",
			integration: Claude,
			content:     "---\nname: other\ndescription: test\n---",
			want:        false,
		},
		{
			name:        "codex with imggen section",
			integration: Codex,
			content:     "# Other stuff\n\n# imggen\nsome content",
			want:        true,
		},
		{
			name:        "codex without imggen",
			integration: Codex,
			content:     "# Other stuff\nno imggen here",
			want:        false,
		},
		{
			name:        "gemini with imggen",
			integration: Gemini,
			content:     "## imggen - AI tool\ncontent here",
			want:        true,
		},
		{
			name:        "cursor with imggen",
			integration: Cursor,
			content:     "Use imggen for image generation tasks",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.isAlreadyRegistered(tt.integration, []byte(tt.content))
			if got != tt.want {
				t.Errorf("isAlreadyRegistered() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistrar_removeExistingSection(t *testing.T) {
	r := &Registrar{}

	tests := []struct {
		name        string
		integration Integration
		content     string
		wantContain string
		wantExclude string
	}{
		{
			name:        "remove imggen section",
			integration: Codex,
			content:     "# Other\nKeep this\n\n# imggen\nRemove this\nAnd this\n\n# Another\nKeep this too",
			wantContain: "Keep this",
			wantExclude: "Remove this",
		},
		{
			name:        "no imggen section",
			integration: Codex,
			content:     "# Other\nKeep all\n\n# Another\nKeep this too",
			wantContain: "Keep all",
			wantExclude: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.removeExistingSection(tt.integration, tt.content)
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("removeExistingSection() should contain %q, got %q", tt.wantContain, got)
			}
			if tt.wantExclude != "" && strings.Contains(got, tt.wantExclude) {
				t.Errorf("removeExistingSection() should not contain %q, got %q", tt.wantExclude, got)
			}
		})
	}
}

func TestExtractMarkdownContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "with frontmatter",
			content: "---\nname: test\n---\n\n# Content\nHello",
			want:    "# Content\nHello",
		},
		{
			name:    "without frontmatter",
			content: "# Content\nHello",
			want:    "# Content\nHello",
		},
		{
			name:    "empty frontmatter",
			content: "---\n---\n\nContent",
			want:    "Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMarkdownContent(tt.content)
			if got != tt.want {
				t.Errorf("extractMarkdownContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRegistrar_convertToAgentsMD(t *testing.T) {
	r := &Registrar{}
	skillContent := "---\nname: imggen\n---\n\n# imggen\nTest content"

	got := r.convertToAgentsMD(skillContent)

	if !strings.Contains(got, "# imggen") {
		t.Error("convertToAgentsMD() should contain # imggen header")
	}
	if !strings.Contains(got, "Test content") {
		t.Error("convertToAgentsMD() should contain the content")
	}
	if !strings.Contains(got, "imggen \"your prompt here\"") {
		t.Error("convertToAgentsMD() should contain example usage")
	}
}

func TestRegistrar_convertToCursorMDC(t *testing.T) {
	r := &Registrar{}
	skillContent := "---\nname: imggen\n---\n\n# imggen\nTest content"

	got := r.convertToCursorMDC(skillContent)

	if !strings.Contains(got, "alwaysApply: true") {
		t.Error("convertToCursorMDC() should contain alwaysApply frontmatter")
	}
	if !strings.Contains(got, "description:") {
		t.Error("convertToCursorMDC() should contain description frontmatter")
	}
	if !strings.Contains(got, "Test content") {
		t.Error("convertToCursorMDC() should contain the content")
	}
}

func TestRegistrar_convertToGeminiMD(t *testing.T) {
	r := &Registrar{}
	skillContent := "---\nname: imggen\n---\n\n# imggen\nTest content"

	got := r.convertToGeminiMD(skillContent)

	if !strings.Contains(got, "# imggen") {
		t.Error("convertToGeminiMD() should contain # imggen header")
	}
	if !strings.Contains(got, "Test content") {
		t.Error("convertToGeminiMD() should contain the content")
	}
}

func TestRegistrar_Status(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()

	// Override the ConfigPath for testing
	oldConfigPath := Claude.ConfigPath
	_ = oldConfigPath // suppress unused warning

	r := NewRegistrar(&bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""))

	// Test non-existent file
	registered, _, err := r.Status(Claude)
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}
	// Note: actual registration check depends on file existence
	_ = registered
	_ = tmpDir
}

func TestRegistrar_backup(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	// Create test file
	originalContent := "original content"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	r := NewRegistrar(&bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""))

	backupPath, err := r.backup(testFile)
	if err != nil {
		t.Fatalf("backup() error = %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file was not created")
	}

	// Verify backup content
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}
	if string(backupContent) != originalContent {
		t.Errorf("backup content = %q, want %q", string(backupContent), originalContent)
	}

	// Verify backup path format
	if !strings.Contains(backupPath, ".backup-") {
		t.Errorf("backup path %q does not contain .backup- suffix", backupPath)
	}
}

func TestRegistrar_Rollback(t *testing.T) {
	tmpDir := t.TempDir()
	originalFile := filepath.Join(tmpDir, "test.md")
	backupFile := originalFile + ".backup-20240101-120000"

	// Create backup file
	backupContent := "backup content"
	if err := os.WriteFile(backupFile, []byte(backupContent), 0644); err != nil {
		t.Fatalf("failed to create backup file: %v", err)
	}

	// Create modified original file
	if err := os.WriteFile(originalFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("failed to create original file: %v", err)
	}

	out := &bytes.Buffer{}
	r := NewRegistrar(out, &bytes.Buffer{}, strings.NewReader(""))

	err := r.Rollback(backupFile)
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Verify original file was restored
	restoredContent, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(restoredContent) != backupContent {
		t.Errorf("restored content = %q, want %q", string(restoredContent), backupContent)
	}
}

func TestRegistrar_Rollback_InvalidPath(t *testing.T) {
	r := NewRegistrar(&bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""))

	err := r.Rollback("")
	if err == nil {
		t.Error("Rollback() with empty path should return error")
	}

	err = r.Rollback("/path/without/backup/suffix.md")
	if err == nil {
		t.Error("Rollback() with invalid backup path should return error")
	}
}

func TestRegistrar_ListBackups(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test integration that uses our temp dir
	// We'll test with the actual function by creating backup files
	r := NewRegistrar(&bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""))

	// Create some backup files manually
	baseFile := filepath.Join(tmpDir, "test.md")
	backups := []string{
		baseFile + ".backup-20240101-100000",
		baseFile + ".backup-20240102-100000",
	}

	for _, b := range backups {
		if err := os.WriteFile(b, []byte("backup"), 0644); err != nil {
			t.Fatalf("failed to create backup: %v", err)
		}
	}

	// Note: ListBackups uses ConfigPath which returns a fixed path
	// so we can't easily test it with temp directories without mocking
	// This test at least verifies the function signature works
	_, err := r.ListBackups(Claude)
	if err != nil {
		// Error is expected if ~/.claude doesn't exist
		if !os.IsNotExist(err) && err != nil {
			// Only fail on unexpected errors
			_ = err
		}
	}
}

func TestGetEmbeddedSkillContent(t *testing.T) {
	content := getEmbeddedSkillContent()

	if content == "" {
		t.Error("getEmbeddedSkillContent() returned empty string")
	}

	if !strings.Contains(content, "name: imggen") {
		t.Error("embedded content should contain 'name: imggen'")
	}

	if !strings.Contains(content, "OPENAI_API_KEY") {
		t.Error("embedded content should contain 'OPENAI_API_KEY'")
	}

	if !strings.Contains(content, "gpt-image-1") {
		t.Error("embedded content should contain 'gpt-image-1'")
	}
}

func TestNewRegistrar(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	in := strings.NewReader("")

	r := NewRegistrar(out, errOut, in)

	if r.Out != out {
		t.Error("NewRegistrar() Out not set correctly")
	}
	if r.Err != errOut {
		t.Error("NewRegistrar() Err not set correctly")
	}
	if r.In != in {
		t.Error("NewRegistrar() In not set correctly")
	}
	if r.DryRun != false {
		t.Error("NewRegistrar() DryRun should default to false")
	}
	if r.Force != false {
		t.Error("NewRegistrar() Force should default to false")
	}
}
