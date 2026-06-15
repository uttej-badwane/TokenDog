# TokenDog — Brand Kit

A vintage engraving / stipple illustration of a Samoyed on warm kraft terracotta.
Friendly, dependable, a little hand-made — the tone of a tool that quietly saves you tokens.

> All assets are derived from the original artwork with the third‑party (Gemini) sparkle
> watermark removed. Nothing here references or depends on any other brand.

---

## Logo system

The brand is **circular**. The primary mark is the round **Badge** — the dog inside a bone
ring — and it's used everywhere the shape can be round: favicon, menu bar, avatars, app
icon, header. The square **Portrait** is the secondary mark for large rectangular space.

| Mark | What it is | Use for |
|------|-----------|---------|
| **Badge** (`logo-badge-*`, circular) | The dog inside a bone ring, round | **Primary** — favicon, menu bar, avatars, app icon, header, anywhere round/small |
| **Portrait** (`logo-square-master-*`) | The dog on terracotta, full bleed | Secondary — docs, slides, README hero, merch, large rectangular space |

Circular assets ship with **transparent corners** so they drop onto any background and read
as a coin. The only deliberately **square** asset is the store app icon (see 03-social).

---

## Colour palette

| Name | Hex | RGB | Role |
|------|-----|-----|------|
| Terracotta | `#C15E3C` | 193, 94, 60 | Primary background, brand fill |
| Clay | `#A94E30` | 169, 78, 48 | Shadows, hover / pressed states |
| Bone | `#ECE6D7` | 236, 230, 215 | Linework paper, the badge ring |
| Warm White | `#F4EFE4` | 244, 239, 228 | Surfaces, text on dark |
| Ink | `#2A2521` | 42, 37, 33 | Body text, outlines |

See `04-swatches-extras/palette.png`.

---

## Typography

- **Wordmark / display:** Baskerville (or any warm transitional serif — Charter, Georgia).
  High-contrast classical serif echoes the engraved illustration.
- **UI / body:** a neutral grotesque (Inter, SF Pro, Helvetica Neue).

The wordmark "TokenDog" is set as one word, no space, capital **T** and **D**.

---

## File index

### 01-website
| File | Size | Purpose |
|------|------|---------|
| `favicon.ico` | 16/32/48/64 | Multi-resolution browser favicon (**circular**, transparent corners) |
| `favicon-16/32/48.png` | — | Small favicons — legibility-tuned (thicker ring, boosted contrast) |
| `favicon-64/128/180/192/512.png` | — | PWA / Apple-touch / Android icons (circular) |
| `header-logo-square-100x100.png` | 100×100 | Round site header mark (circular badge, transparent corners) |
| `header-logo-horizontal-250x150.png` | 250×150 | Horizontal header lockup (badge + wordmark) |
| `header-logo-horizontal@4x-1000x600.png` | 1000×600 | Retina version of the lockup |
| `hero-1920x1080.png` | 1920×1080 | 16:9 hero / banner background |

### 02-logos
| File | Size | Purpose |
|------|------|---------|
| `logo-square-master-500x500.png` | 500×500 | **Primary** square master (portrait) |
| `logo-master-2000x2000.png` | 2000×2000 | High-res master for print / scaling |
| `logo-badge-500x500.png` | 500×500 | Badge mark, standard |
| `logo-badge-2000x2000.png` | 2000×2000 | Badge mark, high-res |

### 03-social
| File | Size | Purpose |
|------|------|---------|
| `profile-400x400.png` | 400×400 | X / Instagram / LinkedIn / Facebook avatar (**circular**, transparent corners) |
| `app-icon-1024x1024.png` | 1024×1024 | iOS App Store / Google Play — **square, full bleed, no alpha** (stores require it & apply their own mask) |
| `app-icon-circular-1024x1024.png` | 1024×1024 | Circular app icon for non-store / in-product display |
| `og-card-1200x630.png` | 1200×630 | Open Graph / Twitter card (link previews) |

### 04-swatches-extras
| File | Purpose |
|------|---------|
| `palette.png` | Colour reference sheet |
| `dog-knockout-transparent.png` | Dog linework on transparent background (overlays, watermarks) |
| `badge-circle-transparent.png` | Circular badge, transparent corners — drops onto any background |

### 05-dark — Ink-background variant set
Every primary asset on **Ink `#2A2521`** instead of Terracotta, for dark UIs, dark
social profiles, and dark-mode favicons. Includes: square + badge masters (500 & 2000),
profile 400, app icon 1024, favicons (16/32/180/512), hero 1920×630… 1080, the horizontal
header lockup (250×150 + @4×), a transparent circular badge, and `og-card-dark-1200x630.png`.

### 06-svg — Vector logos (infinitely scalable)
| File | Purpose |
|------|---------|
| `wordmark-terracotta.svg` / `-bone.svg` / `-ink.svg` | "TokenDog" wordmark in each brand colour |
| `logo-lockup-light.svg` | Circular badge + terracotta wordmark, transparent — for light backgrounds |
| `logo-lockup-dark.svg` | Circular badge (ink) + bone wordmark, transparent — for dark backgrounds |

### 07-menubar — macOS menu-bar icon
Full-colour circular badge, **tuned for small sizes** (thicker bone ring, slightly larger
dog, boosted contrast so the eyes/nose/smile read down to 18 px).
| File | Purpose |
|------|---------|
| `menubar-circle.png` / `@2x` / `@3x` | 18 pt at @1x (18) / @2x (36) / @3x (54) |
| `menubar-circle-22.png`, `-44.png` | 22 px (common bar height) and 44 px hi-res |
| `_preview-18opt.png` | Shown on a bar at 18/22/36 px, plus 4× blow-ups |

> Load as a normal image with `isTemplate = false` (it's colour, not a template). The
> terracotta + bone reads on both light and dark bars. On Retina the `@2x` (36 px) renders;
> the small terracotta gap between the bone ring and the dog is what keeps the badge legible
> at 18 px. A monochrome auto-tinting **template** silhouette can be regenerated on request.

> **SVG note:** wordmarks use a live `<text>` element with a serif font stack
> (`Baskerville → Georgia → Times → serif`). For pixel-perfect production where the
> viewer may lack Baskerville, outline the text to paths in your vector editor.
> `preview.html` (kit root) renders the SVGs against brand backgrounds.

---

## Usage notes

**Do**
- Keep clear space around the mark equal to ~25% of its width.
- Put the badge on Terracotta, Clay, Bone, Ink, or photography with enough contrast.
- Use PNG/ICO as provided; downscale from the largest source for crispness.

**Don't**
- Recolour the dog or stretch any asset off its aspect ratio.
- Re-add a sparkle/star or any third-party glyph.
- Place the bone-ringed badge on a busy light background where the ring disappears.

---

*Sources: original Samoyed artwork (square clean portrait + ringed badge + horizontal scene).
Backgrounds normalised to Terracotta `#C15E3C`; watermark removed.*
