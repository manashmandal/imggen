# imggen

CLI tool for generating images using AI image generation APIs.

## Installation

```bash
go install github.com/manashmandal/imggen/cmd/imggen@latest
```

## Usage

```bash
# Basic usage
imggen "a sunset over mountains"

# With options
imggen -m dall-e-3 -s 1792x1024 -q hd "panoramic cityscape"
imggen -m gpt-image-1 -n 3 --transparent "logo design"
```

## Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--model` | `-m` | Model to use (gpt-image-1, dall-e-3, dall-e-2) | gpt-image-1 |
| `--size` | `-s` | Image size (e.g., 1024x1024) | model default |
| `--quality` | `-q` | Quality level | model default |
| `--count` | `-n` | Number of images | 1 |
| `--output` | `-o` | Output filename | auto-generated |
| `--format` | `-f` | Output format (png, jpeg, webp) | png |
| `--style` | | Style for dall-e-3 (vivid, natural) | |
| `--transparent` | `-t` | Transparent background (gpt-image-1 only) | false |
| `--api-key` | | API key (defaults to OPENAI_API_KEY env var) | |

## Environment Variables

```bash
export OPENAI_API_KEY="your-api-key"
```

## License

MIT
