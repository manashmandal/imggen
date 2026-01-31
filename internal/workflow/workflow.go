package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/manash/imggen/internal/image"
	"github.com/manash/imggen/internal/provider"
	"github.com/manash/imggen/pkg/models"
)

type File struct {
	Workflow Workflow `yaml:"workflow"`
}

type Workflow struct {
	Name   string            `yaml:"name"`
	Params map[string]string `yaml:"params,omitempty"`
	Steps  []Step            `yaml:"steps"`
}

type Step struct {
	ID          string                  `yaml:"id"`
	Prompt      string                  `yaml:"prompt"`
	Model       string                  `yaml:"model,omitempty"`
	Size        string                  `yaml:"size,omitempty"`
	Quality     string                  `yaml:"quality,omitempty"`
	Count       int                     `yaml:"count,omitempty"`
	Format      string                  `yaml:"format,omitempty"`
	References  []models.ReferenceImage `yaml:"references,omitempty"`
	Consistency *models.Consistency     `yaml:"consistency,omitempty"`
	DependsOn   []string                `yaml:"depends_on,omitempty"`
	Outputs     OutputSpec              `yaml:"outputs,omitempty"`
}

type OutputSpec struct {
	Dir     string `yaml:"dir,omitempty"`
	Pattern string `yaml:"pattern,omitempty"`
}

type Engine struct {
	provider provider.Provider
	saver    *image.Saver
	registry *models.ModelRegistry
	out      io.Writer
	err      io.Writer
}

type RunOptions struct {
	OutputDir      string
	DefaultModel   string
	DefaultSize    string
	DefaultQuality string
	DefaultFormat  models.OutputFormat
	Params         map[string]string
}

type StepResult struct {
	StepID string
	Paths  []string
	Cost   float64
}

func NewEngine(prov provider.Provider, saver *image.Saver, registry *models.ModelRegistry, out, errOut io.Writer) *Engine {
	return &Engine{
		provider: prov,
		saver:    saver,
		registry: registry,
		out:      out,
		err:      errOut,
	}
}

func ParseFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}
	workflow := file.Workflow

	if len(workflow.Steps) == 0 {
		if err := yaml.Unmarshal(data, &workflow); err != nil {
			return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
		}
	}

	if len(workflow.Steps) == 0 {
		return nil, fmt.Errorf("workflow has no steps")
	}

	if workflow.Params == nil {
		workflow.Params = map[string]string{}
	}

	if err := workflow.Validate(); err != nil {
		return nil, err
	}

	return &workflow, nil
}

func (w *Workflow) Validate() error {
	seen := map[string]bool{}
	for i, step := range w.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("step %d is missing an id", i+1)
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = true
		if strings.TrimSpace(step.Prompt) == "" {
			return fmt.Errorf("step %q has empty prompt", step.ID)
		}
		if err := models.ValidateReferences(step.References); err != nil {
			return fmt.Errorf("step %q: %w", step.ID, err)
		}
		if step.Consistency != nil {
			if err := step.Consistency.Validate(); err != nil {
				return fmt.Errorf("step %q: %w", step.ID, err)
			}
		}
		if step.Format != "" {
			format := models.OutputFormat(step.Format)
			if !format.IsValid() {
				return fmt.Errorf("step %q has invalid format %q", step.ID, step.Format)
			}
		}
	}

	for _, step := range w.Steps {
		for _, dep := range step.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", step.ID, dep)
			}
		}
	}

	return nil
}

func (e *Engine) Run(ctx context.Context, wf *Workflow, opts *RunOptions) ([]StepResult, error) {
	order, err := topoOrder(wf.Steps)
	if err != nil {
		return nil, err
	}

	params := map[string]string{}
	for k, v := range wf.Params {
		params[k] = v
	}
	for k, v := range opts.Params {
		params[k] = v
	}

	results := make([]StepResult, 0, len(order))

	for _, step := range order {
		prompt, err := applyParams(step.Prompt, params)
		if err != nil {
			return results, fmt.Errorf("step %q: %w", step.ID, err)
		}
		refs := make([]models.ReferenceImage, len(step.References))
		for i, ref := range step.References {
			path, err := applyParams(ref.Path, params)
			if err != nil {
				return results, fmt.Errorf("step %q reference %d: %w", step.ID, i+1, err)
			}
			refPrompt := ref.Prompt
			if refPrompt != "" {
				refPrompt, err = applyParams(refPrompt, params)
				if err != nil {
					return results, fmt.Errorf("step %q reference %d: %w", step.ID, i+1, err)
				}
			}
			refs[i] = models.ReferenceImage{
				Path:   path,
				Prompt: refPrompt,
				Weight: ref.Weight,
			}
		}
		refs = models.NormalizeReferences(refs)

		req := models.NewRequest(prompt)
		req.Model = step.Model
		req.Size = step.Size
		req.Quality = step.Quality
		req.Count = step.Count
		req.Format = opts.DefaultFormat
		req.References = refs
		req.Consistency = step.Consistency

		if req.Model == "" {
			req.Model = opts.DefaultModel
		}
		if req.Size == "" {
			req.Size = opts.DefaultSize
		}
		if req.Quality == "" {
			req.Quality = opts.DefaultQuality
		}
		if req.Count == 0 {
			req.Count = 1
		}
		if step.Format != "" {
			req.Format = models.OutputFormat(step.Format)
		}

		caps, ok := e.registry.Get(req.Model)
		if !ok {
			return results, fmt.Errorf("step %q: unknown model %q", step.ID, req.Model)
		}
		caps.ApplyDefaults(req)

		if err := caps.Validate(req); err != nil {
			return results, fmt.Errorf("step %q: invalid request: %w", step.ID, err)
		}

		resp, err := e.provider.Generate(ctx, req)
		if err != nil {
			return results, fmt.Errorf("step %q: generation failed: %w", step.ID, err)
		}

		outputDir := opts.OutputDir
		if outputDir == "" {
			outputDir = "."
		}
		if step.Outputs.Dir != "" {
			if filepath.IsAbs(step.Outputs.Dir) {
				outputDir = step.Outputs.Dir
			} else {
				outputDir = filepath.Join(outputDir, step.Outputs.Dir)
			}
		}

		paths, err := e.saveStepOutputs(ctx, resp, outputDir, step.Outputs.Pattern, req.Format)
		if err != nil {
			return results, fmt.Errorf("step %q: failed to save outputs: %w", step.ID, err)
		}

		var cost float64
		if resp.Cost != nil {
			cost = resp.Cost.Total
		}

		results = append(results, StepResult{
			StepID: step.ID,
			Paths:  paths,
			Cost:   cost,
		})
	}

	return results, nil
}

func (e *Engine) saveStepOutputs(ctx context.Context, resp *models.Response, outputDir, pattern string, format models.OutputFormat) ([]string, error) {
	paths := make([]string, 0, len(resp.Images))
	total := len(resp.Images)

	if pattern == "" {
		for i := range resp.Images {
			filename := image.GenerateFilename(i, format)
			path := filepath.Join(outputDir, filename)
			if err := e.saver.Save(ctx, &resp.Images[i], path); err != nil {
				return paths, err
			}
			paths = append(paths, path)
		}
		return paths, nil
	}

	hasIndex := strings.Contains(pattern, "{i}")
	ext := filepath.Ext(pattern)
	base := strings.TrimSuffix(pattern, ext)

	for i := range resp.Images {
		index := i + 1
		name := pattern
		if hasIndex {
			name = strings.ReplaceAll(pattern, "{i}", fmt.Sprintf("%d", index))
		} else if total > 1 {
			name = fmt.Sprintf("%s-%d%s", base, index, ext)
		}

		path := filepath.Join(outputDir, name)
		if err := e.saver.Save(ctx, &resp.Images[i], path); err != nil {
			return paths, err
		}
		paths = append(paths, path)
	}

	return paths, nil
}

func topoOrder(steps []Step) ([]Step, error) {
	nodes := map[string]Step{}
	inDegree := map[string]int{}
	deps := map[string][]string{}

	for _, step := range steps {
		nodes[step.ID] = step
		if _, ok := inDegree[step.ID]; !ok {
			inDegree[step.ID] = 0
		}
		for _, dep := range step.DependsOn {
			deps[dep] = append(deps[dep], step.ID)
			inDegree[step.ID]++
		}
	}

	queue := make([]string, 0)
	for id, count := range inDegree {
		if count == 0 {
			queue = append(queue, id)
		}
	}

	order := make([]Step, 0, len(steps))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, nodes[id])
		for _, next := range deps[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(steps) {
		return nil, fmt.Errorf("workflow has a dependency cycle")
	}

	return order, nil
}

var paramPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func applyParams(input string, params map[string]string) (string, error) {
	matches := paramPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	out := input
	for _, match := range matches {
		key := input[match[2]:match[3]]
		key = strings.TrimPrefix(key, "params.")
		val, ok := params[key]
		if !ok {
			return "", fmt.Errorf("missing param %q", key)
		}
		out = strings.ReplaceAll(out, input[match[0]:match[1]], val)
	}
	return out, nil
}
