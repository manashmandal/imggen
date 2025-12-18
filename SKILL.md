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
- Cost information: "Cost: $X.XXXX (N image(s) @ $X.XXXX each)"
- Revised prompt (if returned by API): "Revised prompt: ..."
- Completion message: "Done!"

Generated files are saved to the current working directory with timestamp-based names (e.g., `image-20251216-120000.png`) unless `--output` is specified.

## Cost Tracking

All image generation costs are automatically logged to `~/.imggen/sessions.db`. View costs using the `cost` subcommand:

```bash
# View total costs
imggen cost

# View today's costs
imggen cost today

# View this week's costs (last 7 days)
imggen cost week

# View this month's costs (last 30 days)
imggen cost month

# View costs by provider
imggen cost provider
```

### Interactive Mode Cost Commands

In interactive mode (`imggen -i`), use the `cost` or `$` command:
- `cost today` - Today's costs
- `cost week` - This week's costs
- `cost month` - This month's costs
- `cost total` - All-time total
- `cost provider` - Breakdown by provider
- `cost session` - Current session's costs

## Database Management

Manage the SQLite database storing sessions and cost data:

```bash
# Reset database (delete all data)
imggen db reset

# Reset with backup of old data
imggen db reset --backup

# Show database location and stats
imggen db info
```

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

Costs per image (USD):

### gpt-image-1
| Size | Low | Medium | High |
|------|-----|--------|------|
| 1024x1024 | $0.011 | $0.042 | $0.167 |
| 1536x1024 | $0.016 | $0.063 | $0.250 |
| 1024x1536 | $0.016 | $0.063 | $0.250 |

### dall-e-3
| Size | Standard | HD |
|------|----------|-----|
| 1024x1024 | $0.040 | $0.080 |
| 1024x1792 | $0.080 | $0.120 |
| 1792x1024 | $0.080 | $0.120 |

### dall-e-2
| Size | Cost |
|------|------|
| 256x256 | $0.016 |
| 512x512 | $0.018 |
| 1024x1024 | $0.020 |
