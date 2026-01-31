# Branding Workflow Example

This example showcases imggen's consistency workflow pipeline by generating a branding pack from a single base logo.

## Files

- `base_logo.png`: The reference logo used as the brand anchor.
- `workflow-branding.yaml`: Workflow definition used to generate the assets.
- `outputs/`: Generated assets (logo variants, avatar, business card, letterhead, stickers).

## How it Works

The workflow takes the base logo and produces multiple brand assets while preserving the logo's silhouette and geometry via reference-image consistency.

Steps:
- **logo_variants**: 3 logo variants (invert, outline, simplified mark)
- **social_avatar**: circular avatar with gradient background
- **business_card**: minimal front layout
- **letterhead**: header + watermark
- **sticker_pack**: 3 playful sticker variants

## Run It Yourself

```bash
imggen workflow ./docs/examples/branding/workflow-branding.yaml -o ./docs/examples/branding/outputs
```

You can swap `base_logo.png` with your own logo by editing the `brand_logo` param in the YAML.
