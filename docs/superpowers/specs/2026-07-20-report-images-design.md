# Report Images — Design

**Status:** approved (2026-07-20)

**Goal:** Reconstruct images from session transcripts — screenshots pasted
into prompts and images returned by tools — and show them in the ccwhid
report.

## Motivation

Claude Code stores every image inline in the session JSONL as a content
block:

```json
{"type":"image","source":{"type":"base64","media_type":"image/png","data":"<base64>"}}
```

They occur in two places:

1. directly in a user message's `content` array (pasted/attached
   screenshots), and
2. nested inside `tool_result` content arrays (Read on an image file,
   Playwright screenshots, …).

ccwhid currently drops both silently: `apiBlock` does not decode `source`,
`buildTurn` has no `image` case, and `toolResultText` extracts only text.
Reports lose the images without a trace.

## Decisions

- **Scope:** both placements — user-pasted images and tool-result images.
- **Storage:** decoded to hash-named asset files
  (`assets/images/<sha256-first-16-hex>.<ext>`), referenced via `<img src>`.
  No `data:` URIs — a screenshot-heavy session would inflate `index.html`
  by tens of MB.
- **UI:** inline thumbnail capped at 300px height, click toggles full size.
  No lightbox library.
- **Privacy:** images render by default; `--no-images` drops them
  (placeholder shown instead). Images bypass text redaction — the README
  says so explicitly.

## Design

### Data model (`internal/model`)

- New block type `BlockImage`; `Block` gains `Image *Image`.
- New type:

```go
// Image is one decoded image from the transcript (pasted by the user or
// returned inside a tool result).
type Image struct {
	MediaType string // e.g. "image/png"
	Data      []byte // decoded bytes (not base64)
}
```

- `ToolResult` gains `Images []Image`.

Base64 is decoded once at parse time; the model carries raw bytes.

### Parsing (`internal/transcript`)

- `apiBlock` (raw.go) gains a `Source` field:

```go
Source *apiImageSource `json:"source"`

type apiImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", …
	Data      string `json:"data"`
}
```

- `buildTurn` gets a `case "image"`: decode base64, append a `BlockImage`
  block. Skip the block silently when the source type is not `base64`, the
  media type is unsupported, or the base64 does not decode — same
  degradation style as elsewhere in the parser.
- `toolResultText` is generalized to also collect image blocks from
  tool_result content arrays; the tool_result branch stores them in
  `tc.Result.Images`.
- Supported media types (the set Claude accepts): `image/png`,
  `image/jpeg`, `image/gif`, `image/webp`. Extensions: `.png`, `.jpg`,
  `.gif`, `.webp`.

### Redaction (`internal/redact`)

No change. The redactor scrubs text; it cannot see into pixels. That gap
is the reason the `--no-images` flag exists.

### Rendering (`internal/render`)

- `render.Options` gains `NoImages bool`.
- `Site` walks the session (main turns + all subagent sessions), writes
  each image to `assets/images/<name>` where `<name>` is the first 16 hex
  chars of the SHA-256 of the bytes plus the media-type extension. Hash
  naming dedupes identical images and keeps output stable. The
  `assets/images/` dir is only created when at least one image is written.
- Markup: `<img class="turn-image" src="{Base}assets/images/<name>"
  alt="image/png · 295 KB" loading="lazy">`
  - user-pasted images: a block in the turn body, between the surrounding
    text blocks in transcript order;
  - tool-result images: below the result text inside the tool card.
- CSS: `.turn-image { max-height: 300px; max-width: 100%; cursor:
  zoom-in; }` and `.turn-image.expanded { max-height: none; cursor:
  zoom-out; }`.
- `app.js`: a delegated click handler toggling `expanded` on
  `.turn-image`.
- With `NoImages`, no files are written and each image renders as a muted
  placeholder line instead: `📷 image/png · 295 KB (omitted)`. The same
  `type · size` string doubles as the `alt` text when images are on.

### CLI (`cmd/ccwhid`)

- New `--no-images` flag, plumbed into `render.Options.NoImages`.
- README: short section noting that images bypass redaction — review
  reports before sharing, or use `--no-images`.

## Out of scope

- No image redaction or OCR.
- No thumbnail generation — the browser scales; bytes are written as-is.
- No size caps or sampling.
- No lightbox/zoom beyond the `expanded` class toggle.

## Testing

- **Parse:** tiny valid base64 PNG fixture (1×1 px) in a user message →
  `BlockImage` with decoded bytes and media type; same fixture inside a
  tool_result content array → `Result.Images`; corrupt base64 and
  unsupported media type → block skipped, no error.
- **Render:** hash-named file written under `assets/images/` with correct
  extension; `<img>` src points at it; two identical images produce one
  file; `NoImages` writes no files, creates no `assets/images/` dir, and
  emits the placeholder; subagent-page images resolve via the `../` base
  path.
- **E2E:** transcript JSONL with a pasted image and a tool-result image →
  report contains both `<img>` tags and the decoded file matches the
  fixture bytes.
