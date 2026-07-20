# Report Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reconstruct images from session transcripts — screenshots pasted into prompts and images inside tool results — as hash-named asset files shown inline in the report.

**Architecture:** The parser decodes base64 `image` content blocks into a new `model.Image` (raw bytes + media type), carried on a new `BlockImage` block or in `ToolResult.Images`. The renderer writes each distinct image once to `assets/images/<sha256-16-hex>.<ext>` and emits `<img>` thumbnails (click toggles full size); `--no-images` replaces them with a placeholder line and writes no files.

**Tech Stack:** Go 1.26, stdlib only (crypto/sha256, encoding/base64); tests with plain `testing`.

**Spec:** `docs/superpowers/specs/2026-07-20-report-images-design.md`

## Global Constraints

- Supported media types, exactly: `image/png` → `.png`, `image/jpeg` → `.jpg`, `image/gif` → `.gif`, `image/webp` → `.webp`. Anything else is skipped at parse time.
- An image block is skipped silently (no error, no placeholder) when: `source` missing, `source.type != "base64"`, unsupported media type, base64 does not decode, or decodes to 0 bytes.
- Asset name = first 16 hex chars of SHA-256 of the decoded bytes + extension. `assets/images/` is created only when at least one image is written.
- `--no-images`: no files written, each image renders as `📷 <media type> · <size> (omitted)`; the same `<media type> · <size>` string is the `alt` text when images are on.
- No new dependencies. No regexp. All manual HTML goes through `html.EscapeString`.
- Run `gofmt -l internal/ cmd/` before each commit; it must print nothing.
- Every commit message ends with the two trailer lines shown in Task 1 Step 6 (Co-Authored-By with the implementer's own model name + Claude-Session).

---

### Task 1: Model + parser — decode image blocks

**Files:**
- Modify: `internal/model/model.go` (Block ~line 70, BlockType consts ~line 16, ToolResult ~line 127)
- Modify: `internal/transcript/raw.go` (apiBlock ~line 25, toolResultText ~line 60)
- Modify: `internal/transcript/parse.go` (buildTurn block switch ~line 280)
- Test: `internal/transcript/raw_test.go`, `internal/transcript/parse_test.go`

**Interfaces:**
- Consumes: existing `apiBlock`, `buildTurn`, `model.Block`, `model.ToolResult`.
- Produces (Task 2 relies on these exact names):
  - `model.BlockImage BlockType = "image"`
  - `model.Image { MediaType string; Data []byte }`
  - `model.Block.Image *Image`
  - `model.ToolResult.Images []Image`
  - (internal to transcript) `decodeImage(src *apiImageSource) (model.Image, bool)`, `toolResultParts(raw json.RawMessage) (string, []model.Image)`

- [ ] **Step 1: Write the failing tests**

Append to `internal/transcript/raw_test.go` (add `"bytes"`, `"encoding/base64"`, `"encoding/json"` to its imports if absent; no model import is needed — the tests below only touch fields of inferred values):

```go
// tinyPNGb64 is a valid 1×1 PNG, base64-encoded.
const tinyPNGb64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestDecodeImage(t *testing.T) {
	img, ok := decodeImage(&apiImageSource{Type: "base64", MediaType: "image/png", Data: tinyPNGb64})
	if !ok || img.MediaType != "image/png" {
		t.Fatalf("decodeImage(valid) = %+v, %v", img, ok)
	}
	want, err := base64.StdEncoding.DecodeString(tinyPNGb64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(img.Data, want) {
		t.Error("decoded bytes mismatch")
	}
	for name, src := range map[string]*apiImageSource{
		"nil":          nil,
		"not-base64":   {Type: "url", MediaType: "image/png", Data: tinyPNGb64},
		"bad-media":    {Type: "base64", MediaType: "image/tiff", Data: tinyPNGb64},
		"corrupt-data": {Type: "base64", MediaType: "image/png", Data: "%%%not-base64%%%"},
		"empty-data":   {Type: "base64", MediaType: "image/png", Data: ""},
	} {
		if _, ok := decodeImage(src); ok {
			t.Errorf("decodeImage(%s) unexpectedly ok", name)
		}
	}
}

func TestToolResultParts(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"a"},` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + tinyPNGb64 + `"}},` +
		`{"type":"text","text":"b"}]`)
	text, images := toolResultParts(raw)
	if text != "a\nb" {
		t.Errorf("text = %q, want %q", text, "a\nb")
	}
	if len(images) != 1 || images[0].MediaType != "image/png" || len(images[0].Data) == 0 {
		t.Fatalf("images = %+v", images)
	}
	// Plain-string payloads keep working.
	text, images = toolResultParts(json.RawMessage(`"plain"`))
	if text != "plain" || images != nil {
		t.Errorf("plain string: got %q, %v", text, images)
	}
	// Undecodable image blocks are dropped; text survives.
	raw = json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"%%%"}},{"type":"text","text":"x"}]`)
	text, images = toolResultParts(raw)
	if text != "x" || len(images) != 0 {
		t.Errorf("corrupt image: got %q, %v", text, images)
	}
}
```

Append to `internal/transcript/parse_test.go` (uses the same package; `tinyPNGb64` from raw_test.go is visible; ensure `"os"`, `"path/filepath"`, `"strings"` and the model import are present — they already are in this file):

```go
func TestParseFileImages(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s.jsonl")
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"look"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + tinyPNGb64 + `"}}]},"timestamp":"2026-07-20T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/x.jpg"}}]},"timestamp":"2026-07-20T10:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"` + tinyPNGb64 + `"}}]}]},"timestamp":"2026-07-20T10:00:02Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"%%%corrupt%%%"}},{"type":"text","text":"after"}]},"timestamp":"2026-07-20T10:00:03Z"}`,
	}, "\n")
	if err := os.WriteFile(src, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := ParseFile(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(sess.Turns))
	}
	// Turn 0: text block then image block, in order.
	b := sess.Turns[0].Blocks
	if len(b) != 2 || b[0].Type != model.BlockText || b[1].Type != model.BlockImage {
		t.Fatalf("turn 0 blocks = %+v", b)
	}
	if b[1].Image == nil || b[1].Image.MediaType != "image/png" || len(b[1].Image.Data) == 0 {
		t.Errorf("turn 0 image = %+v", b[1].Image)
	}
	// The tool_result image lands on the Read call's result.
	tc := sess.Turns[1].Blocks[0].Tool
	if tc.Result == nil || len(tc.Result.Images) != 1 || tc.Result.Images[0].MediaType != "image/jpeg" {
		t.Fatalf("tool result = %+v", tc.Result)
	}
	// Corrupt image block is skipped; the text block survives.
	last := sess.Turns[2].Blocks
	if len(last) != 1 || last[0].Type != model.BlockText || last[0].Text != "after" {
		t.Errorf("corrupt-image turn blocks = %+v", last)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transcript/ -run 'TestDecodeImage|TestToolResultParts|TestParseFileImages' -v`
Expected: compile FAIL — `undefined: apiImageSource`, `undefined: decodeImage`, `undefined: toolResultParts`.

- [ ] **Step 3: Implement**

In `internal/model/model.go`, add to the BlockType consts:

```go
	BlockImage    BlockType = "image"
```

Replace the Block struct:

```go
// Block is one content unit: assistant/user text, a thinking block, a tool
// call, or an image pasted into the conversation.
type Block struct {
	Type  BlockType
	Text  string    // for BlockText and BlockThinking
	Tool  *ToolCall // for BlockToolUse
	Image *Image    // for BlockImage
}

// Image is one decoded image from the transcript — pasted by the user or
// returned inside a tool result. Data holds raw bytes, not base64.
type Image struct {
	MediaType string // e.g. "image/png"
	Data      []byte
}
```

Extend ToolResult:

```go
type ToolResult struct {
	Content string
	IsError bool
	Images  []Image // images embedded in the result's content array
}
```

In `internal/transcript/raw.go`, add `"encoding/base64"` and the model import (`"github.com/saigyo/cc-what-have-i-done/internal/model"`) to the imports. Add a `Source` field to `apiBlock`:

```go
type apiBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    *apiImageSource `json:"source"`
}

// apiImageSource mirrors the source object of an image content block.
type apiImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", …
	Data      string `json:"data"`
}
```

Replace `toolResultText` with `toolResultParts` (same position in the file):

```go
// toolResultParts flattens a tool_result content payload into plain text and
// any decodable images.
func toolResultParts(raw json.RawMessage) (string, []model.Image) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []apiBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var parts []string
	var images []model.Image
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
		if img, ok := decodeImage(b.Source); ok {
			images = append(images, img)
		}
	}
	return strings.Join(parts, "\n"), images
}

// imageMediaTypes maps the media types Claude accepts to the file extensions
// the renderer uses; parsing admits only these.
var imageMediaTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
}

// decodeImage decodes an image content block's source. ok is false — and the
// block is skipped — for missing or non-base64 sources, unsupported media
// types, and data that does not decode to at least one byte.
func decodeImage(src *apiImageSource) (model.Image, bool) {
	if src == nil || src.Type != "base64" || !imageMediaTypes[src.MediaType] {
		return model.Image{}, false
	}
	data, err := base64.StdEncoding.DecodeString(src.Data)
	if err != nil || len(data) == 0 {
		return model.Image{}, false
	}
	return model.Image{MediaType: src.MediaType, Data: data}, true
}
```

In `internal/transcript/parse.go`, in `buildTurn`'s block switch, add a case after `"thinking"`:

```go
		case "image":
			if img, ok := decodeImage(b.Source); ok {
				turn.Blocks = append(turn.Blocks, model.Block{Type: model.BlockImage, Image: &img})
			}
```

and change the `"tool_result"` case to use `toolResultParts`:

```go
		case "tool_result":
			if tc := toolIndex[b.ToolUseID]; tc != nil {
				text, images := toolResultParts(b.Content)
				tc.Result = &model.ToolResult{Content: text, IsError: b.IsError, Images: images}
				if tc.IsTaskCreate() && !tc.Result.IsError {
					tc.TaskNumber = taskNumber(tc.Result.Content)
				}
			}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transcript/ ./internal/model/ -v`
Expected: PASS, including all pre-existing tests (`toolResultText` had exactly one caller, replaced above; if the compiler reports another caller, update it the same way and take only the text part).

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add internal/model/model.go internal/transcript/raw.go internal/transcript/parse.go internal/transcript/raw_test.go internal/transcript/parse_test.go
git commit -m "feat(transcript): decode image content blocks into the model

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

### Task 2: Renderer — image assets, thumbnails, --no-images placeholder

**Files:**
- Create: `internal/render/images.go`
- Modify: `internal/render/render.go` (Options ~line 21, Site ~line 94, buildViewModel ~line 203, renderTurnBody ~line 325, renderTool ~line 342 and ~line 371 and ~line 405, turnPlainText ~line 564)
- Modify: `internal/render/assets/styles.css` (append)
- Modify: `internal/render/assets/app.js` (append inside the IIFE)
- Test: `internal/render/images_test.go`

**Interfaces:**
- Consumes: `model.BlockImage`, `model.Image{MediaType, Data}`, `model.Block.Image`, `model.ToolResult.Images` from Task 1.
- Produces: `Options.NoImages bool` (Task 3 plumbs the CLI flag into it); internal `imageFileName(img model.Image) string`, `renderImage(img model.Image, ctx bodyCtx) string`, `writeImages(s model.Session, outDir string) error`, `bodyCtx{links *agentLinks; base string; noImages bool}`.

- [ ] **Step 1: Write the failing tests**

Create `internal/render/images_test.go`:

```go
package render

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// tinyPNGb64 is a valid 1×1 PNG, base64-encoded.
const tinyPNGb64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func testImage(t *testing.T) model.Image {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(tinyPNGb64)
	if err != nil {
		t.Fatal(err)
	}
	return model.Image{MediaType: "image/png", Data: data}
}

func TestImageFileName(t *testing.T) {
	name := imageFileName(model.Image{MediaType: "image/png", Data: []byte{1, 2, 3}})
	if !strings.HasSuffix(name, ".png") || len(name) != 16+len(".png") {
		t.Errorf("name = %q, want 16 hex chars + .png", name)
	}
	if n := imageFileName(model.Image{MediaType: "image/jpeg", Data: []byte{1}}); !strings.HasSuffix(n, ".jpg") {
		t.Errorf("jpeg name = %q, want .jpg suffix", n)
	}
	if n := imageFileName(model.Image{MediaType: "image/tiff", Data: []byte{1}}); n != "" {
		t.Errorf("unsupported media type: name = %q, want empty", n)
	}
	a := imageFileName(model.Image{MediaType: "image/png", Data: []byte{1, 2, 3}})
	b := imageFileName(model.Image{MediaType: "image/png", Data: []byte{1, 2, 3}})
	if a != b {
		t.Error("same bytes must yield the same name")
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{{0, "0 B"}, {999, "999 B"}, {1000, "1 KB"}, {295712, "295 KB"}, {1234567, "1.2 MB"}}
	for _, c := range cases {
		if got := formatBytes(c.n); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestSiteWritesImagesAndTags(t *testing.T) {
	img := testImage(t)
	s := model.Session{Title: "img", Turns: []model.Turn{
		{Kind: model.TurnUser, Blocks: []model.Block{
			{Type: model.BlockText, Text: "look"},
			{Type: model.BlockImage, Image: &img},
		}},
		{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
			ID: "t1", Name: "Read", Summary: "x.png",
			Result: &model.ToolResult{Images: []model.Image{img}},
		}}}},
	}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	name := imageFileName(img)
	data, err := os.ReadFile(filepath.Join(dir, "assets", "images", name))
	if err != nil {
		t.Fatalf("image file missing: %v", err)
	}
	if !bytes.Equal(data, img.Data) {
		t.Error("image bytes mismatch")
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Pasted image + identical tool-result image → two tags, one deduped file.
	if got := strings.Count(string(page), `src="assets/images/`+name+`"`); got != 2 {
		t.Errorf("index has %d references to %s, want 2", got, name)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "assets", "images"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("images dir has %d files, want 1 (dedup)", len(entries))
	}
	if !strings.Contains(string(page), `loading="lazy"`) || !strings.Contains(string(page), `class="turn-image"`) {
		t.Error("img markup missing thumbnail attributes")
	}
}

func TestSiteNoImages(t *testing.T) {
	img := testImage(t)
	s := model.Session{Title: "img", Turns: []model.Turn{
		{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockImage, Image: &img}}},
	}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{NoImages: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "images")); !os.IsNotExist(err) {
		t.Errorf("assets/images must not exist with NoImages (err=%v)", err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "(omitted)") || !strings.Contains(string(page), "image/png") {
		t.Error("placeholder missing")
	}
	if strings.Contains(string(page), `class="turn-image"`) {
		t.Error("unexpected img tag with NoImages")
	}
}

func TestSiteNoImagesInSessionWritesNoImagesDir(t *testing.T) {
	dir := t.TempDir()
	if err := Site(model.Session{Title: "x", Turns: []model.Turn{
		{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
	}}, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "images")); !os.IsNotExist(err) {
		t.Errorf("assets/images must not exist for image-less sessions (err=%v)", err)
	}
}

func TestSiteSubagentPageImageUsesBasePath(t *testing.T) {
	img := testImage(t)
	s := model.Session{Title: "root", Agents: []model.AgentSession{{
		ID: "a1",
		Session: model.Session{Turns: []model.Turn{
			{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockImage, Image: &img}}},
		}},
	}}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "subagents", "agent-a1.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `src="../assets/images/`+imageFileName(img)+`"`) {
		t.Error("subagent page image must use the ../ base prefix")
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "images", imageFileName(img))); err != nil {
		t.Errorf("agent-session image not written: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestImageFileName|TestFormatBytes|TestSite' -v`
Expected: compile FAIL — `undefined: imageFileName`, `unknown field NoImages`.

- [ ] **Step 3: Create `internal/render/images.go`**

```go
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"os"
	"path/filepath"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// imageExt maps a supported media type to its file extension; "" otherwise.
func imageExt(mediaType string) string {
	switch mediaType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ""
}

// imageFileName is the stable asset name for an image: the first 16 hex chars
// of the SHA-256 of its bytes plus the media type's extension. Hash naming
// dedupes identical images and keeps output stable. "" for media types the
// renderer does not know (such images are not rendered).
func imageFileName(img model.Image) string {
	ext := imageExt(img.MediaType)
	if ext == "" {
		return ""
	}
	sum := sha256.Sum256(img.Data)
	return hex.EncodeToString(sum[:8]) + ext
}

// formatBytes renders a byte count as "0 B" / "295 KB" / "1.2 MB" (decimal).
func formatBytes(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%d KB", n/1_000)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// forEachImage calls fn for every image in the session: pasted image blocks,
// tool-result images, images inside nested sidechain turns, and everything in
// linked agent sessions.
func forEachImage(s model.Session, fn func(model.Image)) {
	var walkTurns func(turns []model.Turn)
	walkTurns = func(turns []model.Turn) {
		for _, t := range turns {
			for _, blk := range t.Blocks {
				if blk.Type == model.BlockImage && blk.Image != nil {
					fn(*blk.Image)
				}
				if blk.Type == model.BlockToolUse && blk.Tool != nil {
					if blk.Tool.Result != nil {
						for _, img := range blk.Tool.Result.Images {
							fn(img)
						}
					}
					for _, sub := range blk.Tool.Subagents {
						walkTurns(sub.Turns)
					}
				}
			}
		}
	}
	walkTurns(s.Turns)
	for _, a := range s.Agents {
		walkTurns(a.Session.Turns)
	}
}

// writeImages writes every distinct session image to outDir/assets/images/.
// The directory is only created when there is at least one image to write.
func writeImages(s model.Session, outDir string) error {
	var imgs []model.Image
	seen := map[string]bool{}
	forEachImage(s, func(img model.Image) {
		name := imageFileName(img)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		imgs = append(imgs, img)
	})
	if len(imgs) == 0 {
		return nil
	}
	dir := filepath.Join(outDir, "assets", "images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, img := range imgs {
		if err := os.WriteFile(filepath.Join(dir, imageFileName(img)), img.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// renderImage renders one transcript image: an <img> thumbnail whose click
// toggles full size (app.js), or a muted placeholder when images are off.
func renderImage(img model.Image, ctx bodyCtx) string {
	name := imageFileName(img)
	if name == "" {
		return ""
	}
	label := img.MediaType + " · " + formatBytes(len(img.Data))
	if ctx.noImages {
		return `<div class="image-omitted">📷 ` + html.EscapeString(label) + ` (omitted)</div>`
	}
	return `<img class="turn-image" src="` + html.EscapeString(ctx.base+"assets/images/"+name) +
		`" alt="` + html.EscapeString(label) + `" loading="lazy">`
}
```

- [ ] **Step 4: Wire it into `internal/render/render.go`**

Extend `Options`:

```go
// Options configures a render.
type Options struct {
	Title    string
	Usage    bool   // render the token-usage & cost section
	Version  string // ccwhid build version; shown under the brand ("" → dev build)
	NoImages bool   // omit transcript images; show placeholders instead
}
```

Add a `bodyCtx` type next to `agentLinks` (~line 35):

```go
// bodyCtx carries the page-level context needed while rendering turn bodies.
type bodyCtx struct {
	links    *agentLinks
	base     string // asset prefix: "" on index.html, "../" on agent pages
	noImages bool
}
```

In `Site`, after the styles/app.js asset loop (after ~line 94), write the image files:

```go
	if !opts.NoImages {
		if err := writeImages(s, outDir); err != nil {
			return err
		}
	}
```

In `buildViewModel`, replace the `renderTurnBody(t, links)` call:

```go
			Body:       renderTurnBody(t, bodyCtx{links: links, base: page.Base, noImages: opts.NoImages}),
```

Change `renderTurnBody` and `renderTool` to take the context (three signature changes plus the recursive call):

```go
// renderTurnBody renders all blocks of a turn to HTML.
func renderTurnBody(t model.Turn, ctx bodyCtx) template.HTML {
	var b strings.Builder
	for _, blk := range t.Blocks {
		switch blk.Type {
		case model.BlockText:
			b.WriteString(string(Markdown(blk.Text)))
		case model.BlockThinking:
			b.WriteString(`<details class="thinking"><summary>thinking</summary>`)
			b.WriteString(string(Markdown(blk.Text)))
			b.WriteString(`</details>`)
		case model.BlockImage:
			if blk.Image != nil {
				b.WriteString(renderImage(*blk.Image, ctx))
			}
		case model.BlockToolUse:
			b.WriteString(renderTool(blk.Tool, ctx))
		}
	}
	return template.HTML(b.String())
}
```

In `renderTool`: change the signature to `func renderTool(tc *model.ToolCall, ctx bodyCtx) string`, replace the two internal uses of `links` (`links.forToolUse(tc.ID)` → `ctx.links.forToolUse(tc.ID)`) and the nested-subagent recursion (`renderTurnBody(st, links)` → `renderTurnBody(st, ctx)`). Then, directly after the `if tc.Result != nil && tc.Result.Content != "" && !resultRedundant(tc) { … }` block, add:

```go
	if tc.Result != nil {
		for _, img := range tc.Result.Images {
			b.WriteString(renderImage(img, ctx))
		}
	}
```

In `turnPlainText`, add an image case so prompt previews and the search index aren't empty for image-only messages:

```go
		case model.BlockImage:
			parts = append(parts, "[image]")
```

- [ ] **Step 5: Styles and click-to-expand**

Append to `internal/render/assets/styles.css`:

```css
/* Transcript images */
.turn-image { display: block; max-width: 100%; max-height: 300px; margin: .5rem 0; border: 1px solid var(--border); border-radius: 6px; cursor: zoom-in; }
.turn-image.expanded { max-height: none; cursor: zoom-out; }
.image-omitted { color: var(--muted); font-size: .85rem; margin: .5rem 0; }
```

Append inside the IIFE in `internal/render/assets/app.js` (before the closing `})();`):

```js
  // Transcript images render as capped thumbnails; a click toggles full size.
  document.addEventListener('click', function (e) {
    if (!(e.target instanceof Element)) return;
    var img = e.target.closest('img.turn-image');
    if (img) img.classList.toggle('expanded');
  });
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./...`
Expected: all packages PASS (existing render tests compile against the new signatures because they call `Site`, not `renderTurnBody` directly; if any test calls the old signatures, update the call to pass `bodyCtx{links: links}`).

- [ ] **Step 7: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add internal/render/images.go internal/render/images_test.go internal/render/render.go internal/render/assets/styles.css internal/render/assets/app.js
git commit -m "feat(render): show transcript images as hash-named report assets

Pasted and tool-result images render as 300px thumbnails (click toggles
full size); Options.NoImages swaps them for a muted placeholder and
writes no files.

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

### Task 3: CLI flag, README, end-to-end test

**Files:**
- Modify: `cmd/ccwhid/main.go` (options struct ~line 18, flag block ~line 55)
- Modify: `cmd/ccwhid/main_test.go` (flag list ~line 10)
- Modify: `cmd/ccwhid/run.go:110`
- Modify: `README.md` (flags table ~line 84, Redaction section ~line 91)
- Test: `internal/render/render_e2e_test.go`

**Interfaces:**
- Consumes: `render.Options.NoImages` from Task 2; `transcript.ParseFile`, `redact.Session` (existing).
- Produces: `--no-images` CLI flag.

- [ ] **Step 1: Write the failing tests**

In `cmd/ccwhid/main_test.go`, add `"no-images"` to the flag list in `TestRootCmdHasExpectedFlags`:

```go
	for _, name := range []string{
		"session", "project", "latest", "out", "title",
		"include-subagents", "no-redact", "force", "open",
		"no-images",
	} {
```

Append to `internal/render/render_e2e_test.go` (package `render_test`; imports already cover os/filepath/strings/transcript/redact/render; add `"encoding/base64"` and `"bytes"`):

```go
func TestEndToEndImages(t *testing.T) {
	const tinyPNGb64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	dir := t.TempDir()
	src := filepath.Join(dir, "s.jsonl")
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"see screenshot"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + tinyPNGb64 + `"}}]},"timestamp":"2026-07-20T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/shot.png"}}]},"timestamp":"2026-07-20T10:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + tinyPNGb64 + `"}}]}]},"timestamp":"2026-07-20T10:00:02Z"}`,
	}, "\n")
	if err := os.WriteFile(src, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := transcript.ParseFile(src, transcript.Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	redact.Session(&sess, redact.Config{})
	out := filepath.Join(dir, "report")
	if err := render.Site(sess, out, render.Options{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(out, "assets", "images"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("images dir has %d files, want 1 (same bytes in both places)", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(out, "assets", "images", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	want, err := base64.StdEncoding.DecodeString(tinyPNGb64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Error("written image bytes differ from the fixture")
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(page), `src="assets/images/`+entries[0].Name()+`"`); got != 2 {
		t.Errorf("index references the image %d times, want 2", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ccwhid/ -run TestRootCmdHasExpectedFlags -v && go test ./internal/render/ -run TestEndToEndImages -v`
Expected: flag test FAILs (`expected flag --no-images to be registered`); the e2e test PASSes already (Tasks 1–2 built the pipeline) — it is the regression net for this feature.

- [ ] **Step 3: Implement the flag**

In `cmd/ccwhid/main.go`, add to the options struct:

```go
	noImages         bool
```

and to the flag block:

```go
	f.BoolVar(&opts.noImages, "no-images", false, "omit transcript images (they bypass redaction); show placeholders instead")
```

In `cmd/ccwhid/run.go:110`, pass it through:

```go
	if err := render.Site(sess, outDir, render.Options{Title: opts.title, Usage: opts.usage, Version: version, NoImages: opts.noImages}); err != nil {
```

- [ ] **Step 4: Update README.md**

Add a row to the flags table (after the `--no-redact-name` row):

```markdown
| `--no-images` | Omit transcript images (they bypass redaction); show placeholders instead |
```

At the end of the `## Redaction` section, add:

```markdown
**Images bypass redaction.** Screenshots pasted into prompts and images
returned by tools are copied into the report pixel-for-pixel — the redactor
cannot see into them. Review reports containing images before sharing, or
render with `--no-images`.
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add cmd/ccwhid/main.go cmd/ccwhid/main_test.go cmd/ccwhid/run.go README.md internal/render/render_e2e_test.go
git commit -m "feat(cli): --no-images flag; document that images bypass redaction

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

## Verification (after all tasks)

- `go test ./...` — full suite green; `gofmt -l internal/ cmd/` — no output.
- Manual smoke: `go run ./cmd/ccwhid --session 41f7e8f4 --out /tmp/ccwhid-images-check --force` (that session contains a pasted JPEG and Playwright PNGs in tool results) and confirm: images appear as bordered thumbnails, clicking expands/collapses them, `assets/images/` holds hash-named files, and re-running with `--no-images` shows `📷 image/… (omitted)` lines and writes no image files.
