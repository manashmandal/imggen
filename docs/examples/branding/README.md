# Branding Workflow Example

This example showcases imggen's consistency workflow pipeline by generating a branding pack from a single base logo.

## Files

- `base_logo.png`: The reference logo used as the brand anchor.
- `workflow-branding.yaml`: Workflow definition used to generate the assets.
- `outputs/`: Generated assets (logo variants, avatar, business card, letterhead, stickers).

## Preview

### Base Logo

![Base Logo](./base_logo.png)

### Logo Variants

![Logo Variant 1](./outputs/logo_variant_1.png)
![Logo Variant 2](./outputs/logo_variant_2.png)
![Logo Variant 3](./outputs/logo_variant_3.png)

### Social Avatar

![Avatar](./outputs/avatar.png)

### Business Card

![Business Card](./outputs/business_card_front.png)

### Letterhead

![Letterhead](./outputs/letterhead.png)

### Sticker Pack

![Sticker 1](./outputs/sticker_1.png)
![Sticker 2](./outputs/sticker_2.png)
![Sticker 3](./outputs/sticker_3.png)

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
