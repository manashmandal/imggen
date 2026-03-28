# Workflow Pipelines (YAML)

Workflows allow multi-step image generation with dependencies and parameter substitution.

## CLI Usage

```bash
imggen workflow pipeline.yaml -o ./output \
  --param character_ref=./refs/hero.png
```

## Workflow File Format

Top-level can be either a `workflow` wrapper or direct fields.

```yaml
workflow:
  name: "sprite-pack"
  params:
    character_ref: "./refs/hero.png"
  steps:
    - id: base
      prompt: "full body 2D character, neutral pose"
      references:
        - path: "${character_ref}"
          prompt: "preserve identity and costume"
          weight: 1.0
    - id: run_poses
      depends_on: [base]
      prompt: "same character, running pose variations"
      outputs:
        pattern: "run_{i}.png"
```

## Parameters

Use `${param}` or `${params.param}` in prompts and reference paths.
Provide values via `workflow.params` and override them with `--param key=value`.

## Outputs

- `outputs.dir`: subdirectory for a step (relative to `-o` unless absolute).
- `outputs.pattern`: filename pattern; `{i}` is replaced with 1-based index. If omitted, default timestamped names are used.

## Defaults

When a step omits fields, defaults are taken from CLI flags:
- `--model`, `--size`, `--quality`, `--format`
- `--output-compression` (0-100, GPT image models only)
- `--moderation` ("auto" or "low", GPT image models only)

## Limitations

- Steps run sequentially in dependency order (no parallel execution yet).
- Reference images require models that support edits (gpt-image-1, gpt-image-1-mini, dall-e-2).
