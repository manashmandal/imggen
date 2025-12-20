package register

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

// Integration represents an AI CLI tool that can be registered with imggen
type Integration string

const (
	Claude Integration = "claude"
	Codex  Integration = "codex"
	Cursor Integration = "cursor"
	Gemini Integration = "gemini"
)

// AllIntegrations returns all supported integrations
func AllIntegrations() []Integration {
	return []Integration{Claude, Codex, Cursor, Gemini}
}

// String returns the string representation of the integration
func (i Integration) String() string {
	return string(i)
}

// DisplayName returns a human-readable name for the integration
func (i Integration) DisplayName() string {
	switch i {
	case Claude:
		return "Claude Code"
	case Codex:
		return "OpenAI Codex CLI"
	case Cursor:
		return "Cursor"
	case Gemini:
		return "Gemini CLI"
	default:
		return string(i)
	}
}

// ConfigPath returns the path where the integration config should be written
func (i Integration) ConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	switch i {
	case Claude:
		return filepath.Join(homeDir, ".claude", "skills", "imggen", "SKILL.md"), nil
	case Codex:
		return filepath.Join(homeDir, ".codex", "AGENTS.md"), nil
	case Cursor:
		// Cursor rules are project-local, not global
		// Use current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
		return filepath.Join(cwd, ".cursor", "rules", "imggen.mdc"), nil
	case Gemini:
		return filepath.Join(homeDir, ".gemini", "GEMINI.md"), nil
	default:
		return "", fmt.Errorf("unknown integration: %s", i)
	}
}

// Description returns a brief description of where config is stored
func (i Integration) Description() string {
	switch i {
	case Claude:
		return "~/.claude/skills/imggen/SKILL.md"
	case Codex:
		return "~/.codex/AGENTS.md (appends imggen section)"
	case Cursor:
		return ".cursor/rules/imggen.mdc (project-local)"
	case Gemini:
		return "~/.gemini/GEMINI.md (appends imggen section)"
	default:
		return "unknown"
	}
}

// IsAppendMode returns true if the integration appends to existing config
func (i Integration) IsAppendMode() bool {
	switch i {
	case Codex, Gemini:
		return true
	default:
		return false
	}
}

// Result represents the result of a registration operation
type Result struct {
	Integration  Integration
	Success      bool
	BackupPath   string
	ConfigPath   string
	Error        error
	WasExisting  bool
	WasSkipped   bool
	SkipReason   string
}

// Registrar handles the registration of imggen with AI CLI tools
type Registrar struct {
	Out       io.Writer
	Err       io.Writer
	In        io.Reader
	DryRun    bool
	Force     bool
	SkillPath string // Path to SKILL.md source file
}

// NewRegistrar creates a new Registrar with default settings
func NewRegistrar(out, errOut io.Writer, in io.Reader) *Registrar {
	return &Registrar{
		Out: out,
		Err: errOut,
		In:  in,
	}
}

// Register registers imggen with the specified integrations
func (r *Registrar) Register(integrations []Integration) []Result {
	results := make([]Result, 0, len(integrations))

	for _, integration := range integrations {
		result := r.registerOne(integration)
		results = append(results, result)
	}

	return results
}

func (r *Registrar) registerOne(integration Integration) Result {
	result := Result{Integration: integration}

	configPath, err := integration.ConfigPath()
	if err != nil {
		result.Error = err
		return result
	}
	result.ConfigPath = configPath

	content, err := r.getContent(integration)
	if err != nil {
		result.Error = fmt.Errorf("failed to get content: %w", err)
		return result
	}

	// Check if file exists
	existingContent, err := os.ReadFile(configPath)
	if err == nil {
		result.WasExisting = true

		// Check if imggen is already registered
		if r.isAlreadyRegistered(integration, existingContent) {
			if !r.Force {
				result.WasSkipped = true
				result.SkipReason = "imggen already registered (use --force to overwrite)"
				result.Success = true
				return result
			}
		}
	}

	// Show what will happen
	r.showPreview(integration, configPath, content, result.WasExisting)

	// For Codex, show env config warning early (even in dry-run)
	if integration == Codex {
		needsEnvConfig, _ := r.codexNeedsEnvConfig()
		if needsEnvConfig {
			fmt.Fprintln(r.Out, "")
			fmt.Fprintln(r.Out, "  ⚠️  Codex Environment Configuration Required")
			fmt.Fprintln(r.Out, "  ────────────────────────────────────────────")
			fmt.Fprintln(r.Out, "  imggen requires OPENAI_API_KEY environment variable.")
			fmt.Fprintln(r.Out, "  Codex's sandbox doesn't pass env vars by default.")
			fmt.Fprintln(r.Out, "")
			fmt.Fprintln(r.Out, "  Will add to ~/.codex/config.toml:")
			fmt.Fprintln(r.Out, "    [shell_environment_policy]")
			fmt.Fprintln(r.Out, "    inherit = \"all\"")
			fmt.Fprintln(r.Out, "")
		}
	}

	// Ask for confirmation
	if !r.DryRun && !r.confirm(fmt.Sprintf("Register imggen with %s?", integration.DisplayName())) {
		result.WasSkipped = true
		result.SkipReason = "user cancelled"
		return result
	}

	if r.DryRun {
		result.WasSkipped = true
		result.SkipReason = "dry-run mode"
		result.Success = true
		return result
	}

	// Backup existing file
	if result.WasExisting {
		backupPath, err := r.backup(configPath)
		if err != nil {
			result.Error = fmt.Errorf("failed to create backup: %w", err)
			return result
		}
		result.BackupPath = backupPath
		fmt.Fprintf(r.Out, "  Backup created: %s\n", backupPath)
	}

	// Write the config
	if err := r.writeConfig(integration, configPath, content, existingContent); err != nil {
		result.Error = fmt.Errorf("failed to write config: %w", err)
		return result
	}

	// Handle Codex-specific config.toml for environment variables
	if integration == Codex {
		if err := r.handleCodexEnvConfig(); err != nil {
			result.Error = fmt.Errorf("failed to update Codex config: %w", err)
			return result
		}
	}

	result.Success = true
	fmt.Fprintf(r.Out, "  ✓ Registered with %s\n\n", integration.DisplayName())

	return result
}

func (r *Registrar) getContent(integration Integration) (string, error) {
	// Read SKILL.md content
	skillContent, err := r.readSkillFile()
	if err != nil {
		return "", err
	}

	switch integration {
	case Claude:
		return skillContent, nil
	case Codex:
		return r.convertToAgentsMD(skillContent), nil
	case Cursor:
		return r.convertToCursorMDC(skillContent), nil
	case Gemini:
		return r.convertToGeminiMD(skillContent), nil
	default:
		return "", fmt.Errorf("unknown integration: %s", integration)
	}
}

func (r *Registrar) readSkillFile() (string, error) {
	// Try explicit path first
	if r.SkillPath != "" {
		content, err := os.ReadFile(r.SkillPath)
		if err != nil {
			return "", fmt.Errorf("failed to read SKILL.md from %s: %w", r.SkillPath, err)
		}
		return string(content), nil
	}

	// Try to find SKILL.md in common locations
	paths := []string{
		"SKILL.md",                           // Current directory
		filepath.Join("..", "SKILL.md"),      // Parent directory
		filepath.Join("..", "..", "SKILL.md"), // Grandparent
	}

	// Also check relative to the executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		paths = append(paths,
			filepath.Join(exeDir, "SKILL.md"),
			filepath.Join(exeDir, "..", "SKILL.md"),
			filepath.Join(exeDir, "..", "share", "imggen", "SKILL.md"),
		)
	}

	for _, path := range paths {
		if content, err := os.ReadFile(path); err == nil {
			return string(content), nil
		}
	}

	// Fall back to embedded content
	return getEmbeddedSkillContent(), nil
}

func (r *Registrar) isAlreadyRegistered(integration Integration, content []byte) bool {
	contentStr := string(content)
	switch integration {
	case Claude:
		return strings.Contains(contentStr, "name: imggen")
	case Codex, Gemini:
		return strings.Contains(contentStr, "# imggen") || strings.Contains(contentStr, "## imggen")
	case Cursor:
		return strings.Contains(contentStr, "imggen") && strings.Contains(contentStr, "image generation")
	default:
		return false
	}
}

func (r *Registrar) showPreview(integration Integration, configPath, content string, exists bool) {
	fmt.Fprintf(r.Out, "\n%s:\n", integration.DisplayName())
	fmt.Fprintf(r.Out, "  Config path: %s\n", configPath)
	if exists {
		if integration.IsAppendMode() {
			fmt.Fprintf(r.Out, "  Action: Append imggen section to existing file\n")
		} else {
			fmt.Fprintf(r.Out, "  Action: Replace existing file (backup will be created)\n")
		}
	} else {
		fmt.Fprintf(r.Out, "  Action: Create new file\n")
	}
	fmt.Fprintf(r.Out, "  Content preview:\n")

	// Show first 10 lines
	lines := strings.Split(content, "\n")
	maxLines := 10
	if len(lines) < maxLines {
		maxLines = len(lines)
	}
	for i := 0; i < maxLines; i++ {
		fmt.Fprintf(r.Out, "    %s\n", lines[i])
	}
	if len(lines) > maxLines {
		fmt.Fprintf(r.Out, "    ... (%d more lines)\n", len(lines)-maxLines)
	}
}

func (r *Registrar) confirm(prompt string) bool {
	if !isTerminal(r.In) {
		return true // Non-interactive mode, proceed
	}

	fmt.Fprintf(r.Out, "  %s [Y/n] ", prompt)
	reader := bufio.NewReader(r.In)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "" || response == "y" || response == "yes"
}

func (r *Registrar) backup(configPath string) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := configPath + ".backup-" + timestamp

	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return "", err
	}

	return backupPath, nil
}

func (r *Registrar) writeConfig(integration Integration, configPath, content string, existingContent []byte) error {
	// Ensure parent directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var finalContent string

	if integration.IsAppendMode() && len(existingContent) > 0 {
		// Remove existing imggen section if present, then append
		cleanedContent := r.removeExistingSection(integration, string(existingContent))
		finalContent = cleanedContent + "\n\n" + content
	} else {
		finalContent = content
	}

	return os.WriteFile(configPath, []byte(finalContent), 0644)
}

func (r *Registrar) removeExistingSection(integration Integration, content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inImggenSection := false
	sectionDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of imggen section
		if !inImggenSection {
			if strings.HasPrefix(trimmed, "# imggen") || strings.HasPrefix(trimmed, "## imggen") {
				inImggenSection = true
				sectionDepth = strings.Count(line, "#")
				continue
			}
			result = append(result, line)
		} else {
			// Detect end of imggen section (next section at same or higher level)
			if strings.HasPrefix(trimmed, "#") {
				currentDepth := 0
				for _, c := range trimmed {
					if c == '#' {
						currentDepth++
					} else {
						break
					}
				}
				if currentDepth <= sectionDepth && !strings.Contains(strings.ToLower(trimmed), "imggen") {
					inImggenSection = false
					result = append(result, line)
				}
			}
		}
	}

	// Trim trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n")
}

func (r *Registrar) convertToAgentsMD(skillContent string) string {
	// Extract content after frontmatter (already includes header)
	content := extractMarkdownContent(skillContent)

	return fmt.Sprintf(`%s

## Codex CLI Note

Codex CLI does not pass environment variables to subprocesses ([known bug](https://github.com/openai/codex/issues/6263)).

**Solution**: Use imggen's built-in key storage:
`+"```bash"+`
# One-time setup (run outside Codex)
imggen keys set
# Enter your OpenAI API key when prompted

# Then in Codex, just use:
imggen "your prompt here"
`+"```"+`

The key is stored in ~/.config/imggen/keys.json and used automatically.
`, content)
}

func (r *Registrar) convertToCursorMDC(skillContent string) string {
	// Extract content after frontmatter (already includes header)
	content := extractMarkdownContent(skillContent)

	return fmt.Sprintf(`---
description: Use imggen CLI for AI image generation (DALL-E, gpt-image-1)
globs:
alwaysApply: true
---

%s
`, content)
}

func (r *Registrar) convertToGeminiMD(skillContent string) string {
	// Extract content after frontmatter (already includes header)
	content := extractMarkdownContent(skillContent)
	return content
}

func extractMarkdownContent(skillContent string) string {
	// Remove YAML frontmatter if present
	if strings.HasPrefix(skillContent, "---") {
		parts := strings.SplitN(skillContent, "---", 3)
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[2])
		}
	}
	return strings.TrimSpace(skillContent)
}

func isTerminal(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// Rollback restores a backup file
func (r *Registrar) Rollback(backupPath string) error {
	if backupPath == "" {
		return fmt.Errorf("no backup path provided")
	}

	// Extract original path from backup path
	// Format: /path/to/file.backup-TIMESTAMP
	idx := strings.LastIndex(backupPath, ".backup-")
	if idx == -1 {
		return fmt.Errorf("invalid backup path format: %s", backupPath)
	}
	originalPath := backupPath[:idx]

	content, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := os.WriteFile(originalPath, content, 0644); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	fmt.Fprintf(r.Out, "Restored %s from backup\n", originalPath)
	return nil
}

// ListBackups lists all backup files for an integration
func (r *Registrar) ListBackups(integration Integration) ([]string, error) {
	configPath, err := integration.ConfigPath()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), base+".backup-") {
			backups = append(backups, filepath.Join(dir, entry.Name()))
		}
	}

	return backups, nil
}

// Status returns the registration status for an integration
func (r *Registrar) Status(integration Integration) (registered bool, configPath string, err error) {
	configPath, err = integration.ConfigPath()
	if err != nil {
		return false, "", err
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, configPath, nil
		}
		return false, configPath, err
	}

	return r.isAlreadyRegistered(integration, content), configPath, nil
}

// Unregister removes imggen from an integration
func (r *Registrar) Unregister(integration Integration) error {
	configPath, err := integration.ConfigPath()
	if err != nil {
		return err
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(r.Out, "%s: not registered (config file does not exist)\n", integration.DisplayName())
			return nil
		}
		return err
	}

	if !r.isAlreadyRegistered(integration, content) {
		fmt.Fprintf(r.Out, "%s: not registered\n", integration.DisplayName())
		return nil
	}

	// Show what will happen
	fmt.Fprintf(r.Out, "\n%s:\n", integration.DisplayName())
	fmt.Fprintf(r.Out, "  Config path: %s\n", configPath)

	if integration.IsAppendMode() {
		fmt.Fprintf(r.Out, "  Action: Remove imggen section from file\n")
	} else {
		fmt.Fprintf(r.Out, "  Action: Delete config file\n")
	}

	if !r.DryRun && !r.confirm(fmt.Sprintf("Unregister imggen from %s?", integration.DisplayName())) {
		fmt.Fprintf(r.Out, "  Skipped\n")
		return nil
	}

	if r.DryRun {
		fmt.Fprintf(r.Out, "  Would unregister (dry-run)\n")
		return nil
	}

	// Backup first
	backupPath, err := r.backup(configPath)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Fprintf(r.Out, "  Backup created: %s\n", backupPath)

	if integration.IsAppendMode() {
		// Remove imggen section
		newContent := r.removeExistingSection(integration, string(content))
		if strings.TrimSpace(newContent) == "" {
			// File is empty after removal, delete it
			if err := os.Remove(configPath); err != nil {
				return err
			}
		} else {
			if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
				return err
			}
		}
	} else {
		// Delete the file
		if err := os.Remove(configPath); err != nil {
			return err
		}
	}

	fmt.Fprintf(r.Out, "  ✓ Unregistered from %s\n", integration.DisplayName())
	return nil
}

// Codex config.toml handling

const codexEnvPolicySection = `
[shell_environment_policy]
inherit = "all"
`

// getCodexConfigPath returns the path to Codex's config.toml
func getCodexConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".codex", "config.toml"), nil
}

// codexNeedsEnvConfig checks if Codex config.toml needs shell_environment_policy
func (r *Registrar) codexNeedsEnvConfig() (bool, error) {
	configPath, err := getCodexConfigPath()
	if err != nil {
		return false, err
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // Config doesn't exist, needs to be created
		}
		return false, err
	}

	// Check if shell_environment_policy is already configured
	return !strings.Contains(string(content), "[shell_environment_policy]"), nil
}

// updateCodexEnvConfig adds shell_environment_policy to Codex config.toml
func (r *Registrar) updateCodexEnvConfig() error {
	configPath, err := getCodexConfigPath()
	if err != nil {
		return err
	}

	// Read existing content
	existingContent, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Backup if exists
	if len(existingContent) > 0 {
		backupPath, err := r.backup(configPath)
		if err != nil {
			return fmt.Errorf("failed to backup config.toml: %w", err)
		}
		fmt.Fprintf(r.Out, "  Backup of config.toml: %s\n", backupPath)
	}

	// Append the env policy section
	newContent := string(existingContent) + codexEnvPolicySection

	// Write updated config
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		return err
	}

	fmt.Fprintf(r.Out, "  ✓ Updated %s with shell_environment_policy\n", configPath)
	return nil
}

// handleCodexEnvConfig checks and handles Codex environment config with user consent
func (r *Registrar) handleCodexEnvConfig() error {
	needsConfig, err := r.codexNeedsEnvConfig()
	if err != nil {
		return err
	}

	if !needsConfig {
		return nil // Already configured
	}

	// Warning was already shown in preview, just ask for confirmation
	if !r.confirm("Update ~/.codex/config.toml to pass environment variables?") {
		fmt.Fprintln(r.Out, "")
		fmt.Fprintln(r.Out, "  ⚠️  Without this change, imggen won't work in Codex.")
		fmt.Fprintln(r.Out, "  You can manually add the config later, or run:")
		fmt.Fprintln(r.Out, "    imggen --api-key YOUR_KEY \"prompt\"")
		fmt.Fprintln(r.Out, "")
		return nil // Continue with registration, just warn
	}

	return r.updateCodexEnvConfig()
}
