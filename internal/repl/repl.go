package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/manash/imggen/internal/display"
	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/internal/session"
	"github.com/manash/imggen/pkg/models"
)

type REPL struct {
	in         io.Reader
	out        io.Writer
	err        io.Writer
	provider   provider.Provider
	registry   *models.ModelRegistry
	sessionMgr *session.Manager
	displayer  *display.Displayer
	saver      *image.Saver
	commands   map[string]Command
	running    bool
}

type Config struct {
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	Provider   provider.Provider
	Registry   *models.ModelRegistry
	SessionMgr *session.Manager
	Displayer  *display.Displayer
	Saver      *image.Saver
}

func New(cfg *Config) *REPL {
	r := &REPL{
		in:         cfg.In,
		out:        cfg.Out,
		err:        cfg.Err,
		provider:   cfg.Provider,
		registry:   cfg.Registry,
		sessionMgr: cfg.SessionMgr,
		displayer:  cfg.Displayer,
		saver:      cfg.Saver,
		commands:   make(map[string]Command),
	}
	r.registerCommands()
	return r
}

func (r *REPL) Run(ctx context.Context) error {
	r.running = true
	r.printWelcome()

	scanner := bufio.NewScanner(r.in)
	for r.running {
		r.printPrompt()
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if err := r.execute(ctx, line); err != nil {
			fmt.Fprintf(r.err, "Error: %v\n", err)
		}
	}

	return scanner.Err()
}

func (r *REPL) execute(ctx context.Context, line string) error {
	parts := parseCommand(line)
	if len(parts) == 0 {
		return nil
	}

	cmdName := strings.ToLower(parts[0])
	args := parts[1:]

	cmd, ok := r.commands[cmdName]
	if !ok {
		return fmt.Errorf("unknown command: %s (type 'help' for available commands)", cmdName)
	}

	return cmd.Execute(ctx, r, args)
}

func (r *REPL) Stop() {
	r.running = false
}

func (r *REPL) printWelcome() {
	fmt.Fprintln(r.out, "imggen interactive mode")
	fmt.Fprintln(r.out, "Type 'help' for available commands, 'quit' to exit.")
	fmt.Fprintln(r.out)
}

func (r *REPL) printPrompt() {
	model := r.sessionMgr.GetModel()
	if r.sessionMgr.HasIteration() {
		iter := r.sessionMgr.CurrentIteration()
		fmt.Fprintf(r.out, "imggen [%s] (%s)> ", model, iter.Operation)
	} else {
		fmt.Fprintf(r.out, "imggen [%s]> ", model)
	}
}

func parseCommand(line string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	quoteChar := rune(0)

	for _, ch := range line {
		switch {
		case ch == '"' || ch == '\'':
			if inQuotes && ch == quoteChar {
				inQuotes = false
				quoteChar = 0
			} else if !inQuotes {
				inQuotes = true
				quoteChar = ch
			} else {
				current.WriteRune(ch)
			}
		case ch == ' ' && !inQuotes:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}
