# Reference Images & Consistency

This document explains how to use reference images to keep characters or subjects consistent across generations.

## CLI: Single Prompt

```bash
imggen --ref ./refs/character.png \
  --ref-prompt "preserve identity and costume" \
  --ref-weight 1.0 \
  --consistency-mode identity \
  --consistency-strength 0.8 \
  "same character, running pose"
```

## CLI: Multiple Prompts

When using multiple prompts, the same reference set and consistency settings are applied to each prompt:

```bash
imggen -P "pose one" -P "pose two" \
  --ref ./refs/character.png \
  --ref-prompt "same character" \
  --consistency-mode identity \
  --consistency-strength 0.7 \
  -o ./output
```

## Batch YAML

```yaml
- prompt: "change the background to a forest at dusk"
  references:
    - path: "./refs/character.png"
      prompt: "preserve identity and costume"
      weight: 1.0
  consistency:
    mode: identity
    strength: 0.8
```

## Notes

- Reference images require a model that supports edits (gpt-image-1, gpt-image-1-mini, dall-e-2).
- If a model does not support references, the request fails with a clear error.
- Weights default to 1.0 when omitted.
- Consistency settings are validated: mode must be `identity`, `style`, or `hybrid`, and strength must be 0–1.
