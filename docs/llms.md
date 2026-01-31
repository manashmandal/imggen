# LLM Quickstart

This document is a concise guide for LLMs and automation agents working on imggen.

## What Matters Most

- Primary goal: generate images (and videos) via CLI, with batching and reference-image consistency.
- Reference images are routed through the OpenAI edits endpoint.
- YAML batch and workflow parsing are critical for automated usage.

## Key Entrypoints

- CLI: `cmd/imggen/main.go`
- Image provider: `internal/provider/openai/openai.go`
- Edit/reference support: `internal/provider/openai/edit.go`, `internal/provider/openai/reference.go`
- Models & validation: `pkg/models/models.go`
- Batch parsing: `internal/batch/parser.go`
- Workflow engine: `internal/workflow/workflow.go`

## Reference Images

- Flags: `--ref`, `--ref-prompt`, `--ref-weight`
- Validation: paths required; weights must be non-negative.
- When refs are present, requests go through edits.

## Consistency Controls

- Fields: `consistency.mode` and `consistency.strength`
- Modes: `identity`, `style`, `hybrid`
- Strength range: `0..1`

## Batch / Workflow

- Batch: supports `.txt`, `.json`, `.yaml/.yml`
- Workflow: YAML with steps and optional params; supports `${param}` substitution

## Tests

- Run all tests: `go test ./...`
- Key suites: `internal/provider/openai`, `internal/batch`, `internal/workflow`, `pkg/models`

## Common Pitfalls

- DALL-E 3 does not support references or edits.
- DALL-E 2 supports edits but only a single reference image.
- Ensure request validation before calling provider.
