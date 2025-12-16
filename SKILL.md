---
name: imggen
description: Use this skill when users want to generate images using OpenAI's image generation API (DALL-E or gpt-image-1). Invoke when users request AI-generated images, artwork, logos, illustrations, or visual content from text prompts.
version: 1.0.0
allowed-tools: Bash(imggen:*), Read, Write
model: inherit
---

# imggen - OpenAI Image Generation CLI

Generate images from text prompts using OpenAI's image generation API.

## Overview

`imggen` is a command-line tool that interfaces with OpenAI's image generation API. It supports multiple models (gpt-image-1, dall-e-3, dall-e-2) and provides options for image size, quality, format, and style.

## Prerequisites

- `imggen` binary installed and available in PATH
- `OPENAI_API_KEY` environment variable set with a valid OpenAI API key
- Sufficient OpenAI API credits for image generation

## Usage

```bash
imggen [flags] "prompt"
```

## Available Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--model` | `-m` | `gpt-image-1` | Model: gpt-image-1, dall-e-3, dall-e-2 |
| `--size` | `-s` | `1024x1024` | Image dimensions |
| `--quality` | `-q` | `auto` | Quality level |
| `--count` | `-n` | `1` | Number of images (1-10 for gpt-image-1, 1 for dall-e-3) |
| `--output` | `-o` | auto-generated | Output filename |
| `--format` | `-f` | `png` | Output format: png, jpeg, webp |
| `--style` | | `vivid` | Style for dall-e-3: vivid, natural |
| `--transparent` | `-t` | `false` | Transparent background (gpt-image-1 + png/webp only) |
| `--api-key` | | `$OPENAI_API_KEY` | Override API key |

## Model-Specific Parameters

### gpt-image-1 (Default, Recommended)
- **Sizes**: 1024x1024, 1536x1024 (landscape), 1024x1536 (portrait), auto
- **Quality**: auto, low, medium, high
- **Max images**: 10 per request
- **Supports**: Transparent backgrounds, multiple output formats

### dall-e-3
- **Sizes**: 1024x1024, 1024x1792, 1792x1024
- **Quality**: standard, hd
- **Max images**: 1 per request
- **Supports**: Style parameter (vivid/natural)

### dall-e-2
- **Sizes**: 256x256, 512x512, 1024x1024
- **Max images**: 10 per request

## Instructions

1. Verify `OPENAI_API_KEY` is set in the environment
2. Construct the imggen command with appropriate flags based on user requirements
3. Execute the command using Bash tool
4. Report the generated filename and any revised prompt returned by the API
5. If the user wants to view the image, use Read tool on the generated file

## Output Format

The tool outputs:
- Progress message: "Generating N image(s) with MODEL..."
- Saved filename: "Saved: filename.png"
- Revised prompt (if returned by API): "Revised prompt: ..."
- Completion message: "Done!"

Generated files are saved to the current working directory with timestamp-based names (e.g., `image-20251216-120000.png`) unless `--output` is specified.

## Examples

### Basic image generation
```bash
imggen "a sunset over mountains"
```

### High-quality landscape with DALL-E 3
```bash
imggen -m dall-e-3 -s 1792x1024 -q hd "panoramic view of a futuristic city"
```

### Multiple images with gpt-image-1
```bash
imggen -n 4 -q high "abstract geometric pattern"
```

### Logo with transparent background
```bash
imggen -t -f png "minimalist tech company logo, flat design"
```

### Custom output filename
```bash
imggen -o hero-image.png "website hero banner with gradient"
```

### Natural style portrait
```bash
imggen -m dall-e-3 --style natural "professional headshot, studio lighting"
```

## Error Handling

Common errors and solutions:
- **"API key required"**: Set `OPENAI_API_KEY` environment variable
- **"invalid size"**: Use a size supported by the selected model
- **"supports maximum N images"**: Reduce `--count` value
- **"does not support --style"**: Only dall-e-3 supports style flag
- **"does not support --transparent"**: Only gpt-image-1 supports transparency

## Pricing Reference

Approximate costs per image:
- **gpt-image-1**: ~$0.02 (low), ~$0.07 (medium), ~$0.19 (high) for 1024x1024
- **dall-e-3**: ~$0.04 (standard), ~$0.08 (hd) for 1024x1024
- **dall-e-2**: ~$0.02 for 1024x1024
