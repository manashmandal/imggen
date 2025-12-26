package repl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manash/imggen/internal/security"
	"github.com/manash/imggen/internal/session"
	"github.com/manash/imggen/pkg/models"
)

type Command interface {
	Name() string
	Aliases() []string
	Description() string
	Usage() string
	Execute(ctx context.Context, r *REPL, args []string) error
}

func (r *REPL) registerCommands() {
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
		r.commands[cmd.Name()] = cmd
		for _, alias := range cmd.Aliases() {
			r.commands[alias] = cmd
		}
	}
}

// GenerateCommand generates a new image
type GenerateCommand struct{}

func (c *GenerateCommand) Name() string        { return "generate" }
func (c *GenerateCommand) Aliases() []string   { return []string{"gen", "g"} }
func (c *GenerateCommand) Description() string { return "Generate a new image from a prompt" }
func (c *GenerateCommand) Usage() string       { return "generate <prompt>" }

func (c *GenerateCommand) Execute(ctx context.Context, r *REPL, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s", c.Usage())
	}

	prompt := strings.Join(args, " ")
	model := r.sessionMgr.GetModel()

	req := models.NewRequest(prompt)
	req.Model = model

	caps, ok := r.registry.Get(req.Model)
	if !ok {
		return fmt.Errorf("unknown model: %s", req.Model)
	}
	caps.ApplyDefaults(req)

	fmt.Fprintf(r.out, "Generating with %s...\n", req.Model)

	resp, err := r.provider.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	imagePath := r.sessionMgr.ImagePath()
	paths, err := r.saver.SaveAll(ctx, resp, imagePath, req.Format)
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	var costValue float64
	if resp.Cost != nil {
		costValue = resp.Cost.Total
	}

	iter := &session.Iteration{
		Operation:     "generate",
		Prompt:        prompt,
		RevisedPrompt: resp.RevisedPrompt,
		Model:         req.Model,
		ImagePath:     paths[0],
		Metadata: session.IterationMetadata{
			Size:     req.Size,
			Quality:  req.Quality,
			Format:   req.Format.String(),
			Cost:     costValue,
			Provider: string(r.provider.Name()),
		},
	}
	if err := r.sessionMgr.AddIteration(ctx, iter); err != nil {
		return fmt.Errorf("failed to save iteration: %w", err)
	}

	// Log cost to database
	if resp.Cost != nil && resp.Cost.Total > 0 {
		costEntry := &session.CostEntry{
			IterationID: iter.ID,
			SessionID:   r.sessionMgr.Current().ID,
			Provider:    string(r.provider.Name()),
			Model:       req.Model,
			Cost:        resp.Cost.Total,
			ImageCount:  len(resp.Images),
			Timestamp:   iter.Timestamp,
		}
		if err := r.sessionMgr.LogCost(ctx, costEntry); err != nil {
			fmt.Fprintf(r.err, "Warning: failed to log cost: %v\n", err)
		}
	}

	if err := r.displayer.Display(ctx, &resp.Images[0]); err != nil {
		fmt.Fprintf(r.err, "Warning: failed to display: %v\n", err)
	}

	fmt.Fprintf(r.out, "Saved: %s\n", paths[0])
	if resp.Cost != nil {
		fmt.Fprintf(r.out, "Cost: $%.4f (%d image(s) @ $%.4f/image, %s %s %s)\n",
			resp.Cost.Total, len(resp.Images), resp.Cost.PerImage,
			req.Model, req.Size, req.Quality)
	}
	if resp.RevisedPrompt != "" {
		fmt.Fprintf(r.out, "Revised prompt: %s\n", resp.RevisedPrompt)
	}

	return nil
}

// EditCommand edits the current image
type EditCommand struct{}

func (c *EditCommand) Name() string        { return "edit" }
func (c *EditCommand) Aliases() []string   { return []string{"e"} }
func (c *EditCommand) Description() string { return "Edit the current image with a prompt" }
func (c *EditCommand) Usage() string       { return "edit <prompt>" }

func (c *EditCommand) Execute(ctx context.Context, r *REPL, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s", c.Usage())
	}

	if !r.sessionMgr.HasIteration() {
		return fmt.Errorf("no current image - use 'generate' first")
	}

	model := r.sessionMgr.GetModel()
	if !r.provider.SupportsEdit(model) {
		return fmt.Errorf("model %s does not support editing", model)
	}

	currentPath := r.sessionMgr.CurrentImagePath()
	imageData, err := os.ReadFile(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read current image: %w", err)
	}

	prompt := strings.Join(args, " ")

	req := models.NewEditRequest(imageData, prompt)
	req.Model = model

	caps, ok := r.registry.Get(req.Model)
	if ok {
		if req.Size == "" {
			req.Size = caps.DefaultSize
		}
	}

	fmt.Fprintf(r.out, "Editing with %s...\n", req.Model)

	resp, err := r.provider.Edit(ctx, req)
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	imagePath := r.sessionMgr.ImagePath()
	paths, err := r.saver.SaveAll(ctx, resp, imagePath, req.Format)
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	var costValue float64
	if resp.Cost != nil {
		costValue = resp.Cost.Total
	}

	iter := &session.Iteration{
		Operation:     "edit",
		Prompt:        prompt,
		RevisedPrompt: resp.RevisedPrompt,
		Model:         req.Model,
		ImagePath:     paths[0],
		Metadata: session.IterationMetadata{
			Size:     req.Size,
			Format:   req.Format.String(),
			Cost:     costValue,
			Provider: string(r.provider.Name()),
		},
	}
	if err := r.sessionMgr.AddIteration(ctx, iter); err != nil {
		return fmt.Errorf("failed to save iteration: %w", err)
	}

	// Log cost to database
	if resp.Cost != nil && resp.Cost.Total > 0 {
		costEntry := &session.CostEntry{
			IterationID: iter.ID,
			SessionID:   r.sessionMgr.Current().ID,
			Provider:    string(r.provider.Name()),
			Model:       req.Model,
			Cost:        resp.Cost.Total,
			ImageCount:  len(resp.Images),
			Timestamp:   iter.Timestamp,
		}
		if err := r.sessionMgr.LogCost(ctx, costEntry); err != nil {
			fmt.Fprintf(r.err, "Warning: failed to log cost: %v\n", err)
		}
	}

	if err := r.displayer.Display(ctx, &resp.Images[0]); err != nil {
		fmt.Fprintf(r.err, "Warning: failed to display: %v\n", err)
	}

	fmt.Fprintf(r.out, "Saved: %s\n", paths[0])
	if resp.Cost != nil {
		// Edit uses medium quality for gpt-image-1, empty for dall-e-2
		quality := "medium"
		if req.Model == "dall-e-2" {
			quality = ""
		}
		if quality != "" {
			fmt.Fprintf(r.out, "Cost: $%.4f (%d image(s) @ $%.4f/image, %s %s %s)\n",
				resp.Cost.Total, len(resp.Images), resp.Cost.PerImage,
				req.Model, req.Size, quality)
		} else {
			fmt.Fprintf(r.out, "Cost: $%.4f (%d image(s) @ $%.4f/image, %s %s)\n",
				resp.Cost.Total, len(resp.Images), resp.Cost.PerImage,
				req.Model, req.Size)
		}
	}
	if resp.RevisedPrompt != "" {
		fmt.Fprintf(r.out, "Revised prompt: %s\n", resp.RevisedPrompt)
	}

	return nil
}

// UndoCommand reverts to the previous iteration
type UndoCommand struct{}

func (c *UndoCommand) Name() string        { return "undo" }
func (c *UndoCommand) Aliases() []string   { return []string{"u", "back"} }
func (c *UndoCommand) Description() string { return "Revert to the previous iteration" }
func (c *UndoCommand) Usage() string       { return "undo" }

func (c *UndoCommand) Execute(ctx context.Context, r *REPL, _ []string) error {
	prev, err := r.sessionMgr.Undo(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(r.out, "Reverted to: %s\n", prev.Prompt)

	imageData, err := os.ReadFile(prev.ImagePath)
	if err == nil {
		img := &models.GeneratedImage{Data: imageData}
		if err := r.displayer.Display(ctx, img); err != nil {
			fmt.Fprintf(r.err, "Warning: failed to display: %v\n", err)
		}
	}

	return nil
}

// SaveCommand saves the current image to a specified path
type SaveCommand struct{}

func (c *SaveCommand) Name() string        { return "save" }
func (c *SaveCommand) Aliases() []string   { return []string{"s"} }
func (c *SaveCommand) Description() string { return "Save current image to a file" }
func (c *SaveCommand) Usage() string       { return "save [filename]" }

func (c *SaveCommand) Execute(_ context.Context, r *REPL, args []string) error {
	if !r.sessionMgr.HasIteration() {
		return fmt.Errorf("no current image to save")
	}

	currentPath := r.sessionMgr.CurrentImagePath()

	var destPath string
	if len(args) > 0 {
		destPath = args[0]
		// Validate path to prevent path traversal attacks
		if err := security.ValidateSavePath(destPath); err != nil {
			return fmt.Errorf("invalid save path: %w", err)
		}
	} else {
		destPath = filepath.Base(currentPath)
	}

	data, err := os.ReadFile(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read image: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	fmt.Fprintf(r.out, "Saved: %s\n", destPath)
	return nil
}

// ShowCommand displays the current image
type ShowCommand struct{}

func (c *ShowCommand) Name() string        { return "show" }
func (c *ShowCommand) Aliases() []string   { return []string{"display", "view"} }
func (c *ShowCommand) Description() string { return "Display the current image" }
func (c *ShowCommand) Usage() string       { return "show" }

func (c *ShowCommand) Execute(ctx context.Context, r *REPL, _ []string) error {
	if !r.sessionMgr.HasIteration() {
		return fmt.Errorf("no current image to display")
	}

	currentPath := r.sessionMgr.CurrentImagePath()
	imageData, err := os.ReadFile(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read image: %w", err)
	}

	img := &models.GeneratedImage{Data: imageData}
	return r.displayer.Display(ctx, img)
}

// HistoryCommand shows iteration history
type HistoryCommand struct{}

func (c *HistoryCommand) Name() string        { return "history" }
func (c *HistoryCommand) Aliases() []string   { return []string{"h", "hist"} }
func (c *HistoryCommand) Description() string { return "Show iteration history" }
func (c *HistoryCommand) Usage() string       { return "history" }

func (c *HistoryCommand) Execute(ctx context.Context, r *REPL, _ []string) error {
	history, err := r.sessionMgr.History(ctx)
	if err != nil {
		return err
	}

	if len(history) == 0 {
		fmt.Fprintln(r.out, "No history yet")
		return nil
	}

	currentID := ""
	if r.sessionMgr.HasIteration() {
		currentID = r.sessionMgr.CurrentIteration().ID
	}

	for i, iter := range history {
		marker := "  "
		if iter.ID == currentID {
			marker = "> "
		}
		fmt.Fprintf(r.out, "%s[%d] %s %s: %q\n",
			marker,
			i+1,
			session.FormatTimestamp(iter.Timestamp),
			iter.Operation,
			truncate(iter.Prompt, 50))
	}

	return nil
}

// SessionCommand manages sessions
type SessionCommand struct{}

func (c *SessionCommand) Name() string        { return "session" }
func (c *SessionCommand) Aliases() []string   { return []string{"sess"} }
func (c *SessionCommand) Description() string { return "Manage sessions (list, load, new, rename)" }
func (c *SessionCommand) Usage() string       { return "session <list|load|new|rename> [args]" }

func (c *SessionCommand) Execute(ctx context.Context, r *REPL, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s", c.Usage())
	}

	subCmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch subCmd {
	case "list", "ls":
		return c.list(ctx, r)
	case "load":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: session load <id>")
		}
		return c.load(ctx, r, subArgs[0])
	case "new":
		name := ""
		if len(subArgs) > 0 {
			name = strings.Join(subArgs, " ")
		}
		return c.new(ctx, r, name)
	case "rename":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: session rename <name>")
		}
		return c.rename(ctx, r, strings.Join(subArgs, " "))
	default:
		return fmt.Errorf("unknown session command: %s", subCmd)
	}
}

func (c *SessionCommand) list(ctx context.Context, r *REPL) error {
	sessions, err := r.sessionMgr.ListSessions(ctx)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Fprintln(r.out, "No sessions found")
		return nil
	}

	currentID := ""
	if r.sessionMgr.HasSession() {
		currentID = r.sessionMgr.Current().ID
	}

	fmt.Fprintf(r.out, "%-8s  %-20s  %-20s  %s\n", "ID", "Name", "Updated", "Model")
	fmt.Fprintln(r.out, strings.Repeat("-", 70))

	for _, sess := range sessions {
		marker := "  "
		if sess.ID == currentID {
			marker = "> "
		}
		name := sess.Name
		if name == "" {
			name = "(unnamed)"
		}
		fmt.Fprintf(r.out, "%s%-6s  %-20s  %-20s  %s\n",
			marker,
			sess.ID[:6],
			truncate(name, 20),
			session.FormatTimestamp(sess.UpdatedAt),
			sess.Model)
	}

	return nil
}

func (c *SessionCommand) load(ctx context.Context, r *REPL, id string) error {
	sessions, err := r.sessionMgr.ListSessions(ctx)
	if err != nil {
		return err
	}

	var fullID string
	for _, sess := range sessions {
		if strings.HasPrefix(sess.ID, id) {
			fullID = sess.ID
			break
		}
	}

	if fullID == "" {
		return fmt.Errorf("session not found: %s", id)
	}

	if err := r.sessionMgr.Load(ctx, fullID); err != nil {
		return err
	}

	sess := r.sessionMgr.Current()
	name := sess.Name
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Fprintf(r.out, "Loaded session: %s (%s)\n", name, sess.ID[:6])

	if r.sessionMgr.HasIteration() {
		iter := r.sessionMgr.CurrentIteration()
		fmt.Fprintf(r.out, "Current: %s - %q\n", iter.Operation, truncate(iter.Prompt, 50))
	}

	return nil
}

func (c *SessionCommand) new(ctx context.Context, r *REPL, name string) error {
	sess, err := r.sessionMgr.StartNew(ctx, name)
	if err != nil {
		return err
	}

	displayName := name
	if displayName == "" {
		displayName = "(unnamed)"
	}
	fmt.Fprintf(r.out, "Created new session: %s (%s)\n", displayName, sess.ID[:6])
	return nil
}

func (c *SessionCommand) rename(ctx context.Context, r *REPL, name string) error {
	if err := r.sessionMgr.RenameSession(ctx, name); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Session renamed to: %s\n", name)
	return nil
}

// ModelCommand changes the current model
type ModelCommand struct{}

func (c *ModelCommand) Name() string        { return "model" }
func (c *ModelCommand) Aliases() []string   { return []string{"m"} }
func (c *ModelCommand) Description() string { return "Get or set the current model" }
func (c *ModelCommand) Usage() string       { return "model [name]" }

func (c *ModelCommand) Execute(_ context.Context, r *REPL, args []string) error {
	if len(args) == 0 {
		model := r.sessionMgr.GetModel()
		caps, ok := r.registry.Get(model)
		supportsEdit := "no"
		if ok && caps.SupportsEdit {
			supportsEdit = "yes"
		}
		fmt.Fprintf(r.out, "Current model: %s (edit support: %s)\n", model, supportsEdit)
		fmt.Fprintln(r.out, "\nAvailable models:")
		for _, name := range r.registry.List() {
			cap, _ := r.registry.Get(name)
			edit := ""
			if cap.SupportsEdit {
				edit = " [edit]"
			}
			fmt.Fprintf(r.out, "  - %s (%s)%s\n", name, cap.Provider, edit)
		}
		return nil
	}

	modelName := args[0]
	if _, ok := r.registry.Get(modelName); !ok {
		return fmt.Errorf("unknown model: %s", modelName)
	}

	r.sessionMgr.SetModel(modelName)
	fmt.Fprintf(r.out, "Model set to: %s\n", modelName)
	return nil
}

// CostCommand displays cost information
type CostCommand struct{}

func (c *CostCommand) Name() string        { return "cost" }
func (c *CostCommand) Aliases() []string   { return []string{"$"} }
func (c *CostCommand) Description() string { return "View cost summary (today, week, month, total, provider, session)" }
func (c *CostCommand) Usage() string       { return "cost <today|week|month|total|provider|session>" }

func (c *CostCommand) Execute(ctx context.Context, r *REPL, args []string) error {
	if len(args) == 0 {
		return c.showTotal(ctx, r)
	}

	subCmd := strings.ToLower(args[0])
	switch subCmd {
	case "today":
		return c.showToday(ctx, r)
	case "week":
		return c.showWeek(ctx, r)
	case "month":
		return c.showMonth(ctx, r)
	case "total":
		return c.showTotal(ctx, r)
	case "provider":
		return c.showByProvider(ctx, r)
	case "session":
		return c.showSession(ctx, r)
	default:
		return fmt.Errorf("unknown cost command: %s\nUsage: %s", subCmd, c.Usage())
	}
}

func (c *CostCommand) showToday(ctx context.Context, r *REPL) error {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)

	summary, err := r.sessionMgr.GetCostByDateRange(ctx, start, end)
	if err != nil {
		return err
	}

	if summary.EntryCount == 0 {
		fmt.Fprintln(r.out, "No costs recorded today.")
		return nil
	}

	fmt.Fprintf(r.out, "Today's cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)
	return nil
}

func (c *CostCommand) showWeek(ctx context.Context, r *REPL) error {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(-6 * 24 * time.Hour)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)

	summary, err := r.sessionMgr.GetCostByDateRange(ctx, start, end)
	if err != nil {
		return err
	}

	if summary.EntryCount == 0 {
		fmt.Fprintln(r.out, "No costs recorded in the last 7 days.")
		return nil
	}

	fmt.Fprintf(r.out, "Last 7 days cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)
	return nil
}

func (c *CostCommand) showMonth(ctx context.Context, r *REPL) error {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(-29 * 24 * time.Hour)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)

	summary, err := r.sessionMgr.GetCostByDateRange(ctx, start, end)
	if err != nil {
		return err
	}

	if summary.EntryCount == 0 {
		fmt.Fprintln(r.out, "No costs recorded in the last 30 days.")
		return nil
	}

	fmt.Fprintf(r.out, "Last 30 days cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)
	return nil
}

func (c *CostCommand) showTotal(ctx context.Context, r *REPL) error {
	summary, err := r.sessionMgr.GetTotalCost(ctx)
	if err != nil {
		return err
	}

	if summary.EntryCount == 0 {
		fmt.Fprintln(r.out, "No costs recorded yet.")
		return nil
	}

	fmt.Fprintf(r.out, "Total cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)
	return nil
}

func (c *CostCommand) showByProvider(ctx context.Context, r *REPL) error {
	summaries, err := r.sessionMgr.GetCostByProvider(ctx)
	if err != nil {
		return err
	}

	if len(summaries) == 0 {
		fmt.Fprintln(r.out, "No costs recorded yet.")
		return nil
	}

	fmt.Fprintf(r.out, "%-12s  %-8s  %s\n", "Provider", "Images", "Cost")
	fmt.Fprintln(r.out, strings.Repeat("-", 35))

	var totalCost float64
	var totalImages int
	for _, ps := range summaries {
		fmt.Fprintf(r.out, "%-12s  %-8d  $%.4f\n", ps.Provider, ps.ImageCount, ps.TotalCost)
		totalCost += ps.TotalCost
		totalImages += ps.ImageCount
	}

	fmt.Fprintln(r.out, strings.Repeat("-", 35))
	fmt.Fprintf(r.out, "%-12s  %-8d  $%.4f\n", "Total", totalImages, totalCost)

	return nil
}

func (c *CostCommand) showSession(ctx context.Context, r *REPL) error {
	if !r.sessionMgr.HasSession() {
		fmt.Fprintln(r.out, "No active session.")
		return nil
	}

	summary, err := r.sessionMgr.GetSessionCost(ctx)
	if err != nil {
		return err
	}

	if summary.EntryCount == 0 {
		fmt.Fprintln(r.out, "No costs in current session.")
		return nil
	}

	fmt.Fprintf(r.out, "Session cost: $%.4f (%d image(s))\n", summary.TotalCost, summary.ImageCount)
	return nil
}

// HelpCommand shows available commands
type HelpCommand struct{}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Aliases() []string   { return []string{"?"} }
func (c *HelpCommand) Description() string { return "Show available commands" }
func (c *HelpCommand) Usage() string       { return "help" }

func (c *HelpCommand) Execute(_ context.Context, r *REPL, _ []string) error {
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

	fmt.Fprintln(r.out, "Available commands:")
	fmt.Fprintln(r.out)

	for _, cmd := range commands {
		aliases := ""
		if len(cmd.Aliases()) > 0 {
			aliases = fmt.Sprintf(" (%s)", strings.Join(cmd.Aliases(), ", "))
		}
		fmt.Fprintf(r.out, "  %-12s%s\n", cmd.Name()+aliases, cmd.Description())
		fmt.Fprintf(r.out, "               Usage: %s\n", cmd.Usage())
	}

	return nil
}

// QuitCommand exits the REPL
type QuitCommand struct{}

func (c *QuitCommand) Name() string        { return "quit" }
func (c *QuitCommand) Aliases() []string   { return []string{"exit", "q"} }
func (c *QuitCommand) Description() string { return "Exit interactive mode" }
func (c *QuitCommand) Usage() string       { return "quit" }

func (c *QuitCommand) Execute(_ context.Context, r *REPL, _ []string) error {
	fmt.Fprintln(r.out, "Goodbye!")
	r.Stop()
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
