# LLM Quickstart

This document is a concise guide for LLMs and automation agents working on imggen.

## What Matters Most

- Primary goal: generate images, edit images, analyze images, and generate videos via CLI, with batching and reference-image consistency.
- Reference images are routed through the OpenAI edits endpoint.
- YAML batch and workflow parsing are critical for automated usage.

## Key Entrypoints

- CLI: `cmd/imggen/main.go`
- Image provider: `internal/provider/openai/openai.go`
- Edit/reference support: `internal/provider/openai/edit.go`, `internal/provider/openai/reference.go`
- Models & validation: `pkg/models/models.go`
- Batch parsing: `internal/batch/parser.go`
- Workflow engine: `internal/workflow/workflow.go`
- Image editing command: `cmd/imggen/main.go` (runEdit function)
- Image describe command: `cmd/imggen/main.go` (runDescribe function)
- Responses API provider: `internal/provider/openai/responses.go`
- Responses API types: `pkg/models/responses.go`
- Provider interfaces: `internal/provider/provider.go` (Provider, VideoProvider, OCRProvider, ResponsesProvider)
- REPL commands: `internal/repl/commands.go`

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
- Sora video generation is shutting down September 24, 2026. Do not build new features on Sora.

## New Features (v1.2.0)

### Image Editing
- `imggen edit <image> [prompt]` - Edit images with text instructions
- Supports `--mask` for inpainting, `--bg-remove` for background removal
- Uses OpenAI's `/v1/images/edits` endpoint

### Image Describe
- `imggen describe <image...>` - Vision-based image analysis
- Supports multi-image comparison
- Reuses OCR provider with custom prompts

### Responses API
- Multi-turn image generation via OpenAI's `/v1/responses` endpoint
- Integrated into REPL: subsequent generates reference previous context
- Falls back to Images API if Responses API fails
- Provider: `internal/provider/openai/responses.go`
- Supports tool-level model override: the image generation tool within a Responses API call can specify a different model than the conversation model

### Models

Supported image models:
- `gpt-image-1.5` (default)
- `gpt-image-1`
- `gpt-image-1-mini` — same capabilities as gpt-image-1 (transparency, editing, same sizes/qualities), 50-70% cheaper
- `dall-e-3`
- `dall-e-2`

### Additional Parameters (GPT Image Models Only)

- `output_compression`: int 0-100, controls compression level for jpeg/webp output. Applies to gpt-image-1, gpt-image-1-mini, and gpt-image-1.5.
- `moderation`: `"auto"` or `"low"`, controls content moderation strictness. Applies to gpt-image-1, gpt-image-1-mini, and gpt-image-1.5.
