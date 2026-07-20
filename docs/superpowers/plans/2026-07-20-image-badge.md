# Tool-Card Image Badge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapsed tool cards show a small `📷` / `📷 N` badge in their header when their hidden content contains images.

**Architecture:** A shared per-tool-call image walker (`forEachToolCallImage`) is extracted from `forEachImage` and reused by a new `toolImageCount`; `imageBadge` renders the header span from the count, and `renderTool` emits it between the summary and the transcript link. CSS pins badge and agent-link (`flex-shrink: 0`) and nudges the emoji per rendering engine via `@supports`.

**Tech Stack:** Go 1.26, stdlib only; tests with plain `testing`.

**Spec:** `docs/superpowers/specs/2026-07-20-image-badge-design.md`

## Global Constraints

- Badge markup, exactly: `<span class="image-badge"><span class="image-badge-icon">📷</span></span>` for count 1; `<span class="image-badge"><span class="image-badge-icon">📷</span> N</span>` for N > 1; nothing for 0.
- Counted images = the card's own `Result.Images` plus every image inside `tc.Subagents` turns, recursively (sidechain turns can contain tool calls with their own images and sidechains). Linked agent sessions (`Session.Agents`) are NOT counted.
- The badge shows regardless of `Options.NoImages`.
- `forEachImage`'s observable behavior must not change (asset writing identical before/after refactor).
- No new dependencies. No regexp. All manual HTML goes through `html.EscapeString` (the badge itself contains only our own literals and `strconv.Itoa` output).
- Run `gofmt -l internal/ cmd/` before each commit; it must print nothing.
- Every commit message ends with the two trailer lines shown in Task 1 Step 5 (Co-Authored-By with the implementer's own model name + Claude-Session).

---

### Task 1: Per-tool-call image walker + count

**Files:**
- Modify: `internal/render/images.go` (forEachImage ~line 57; new funcs after it)
- Test: `internal/render/images_test.go`

**Interfaces:**
- Consumes: `model.ToolCall{Result *ToolResult; Subagents []Subagent}`, `model.ToolResult.Images []Image`, `model.Subagent.Turns []Turn`, `model.BlockImage`, `model.BlockToolUse` (all existing).
- Produces: `forEachToolCallImage(tc *model.ToolCall, fn func(model.Image))` and `toolImageCount(tc *model.ToolCall) int` — Task 2 calls `toolImageCount`.

- [ ] **Step 1: Write the failing test**

Append to `internal/render/images_test.go` (the file already imports `model` and `testing`, and defines `testImage`):

```go
func TestToolImageCount(t *testing.T) {
	img := testImage(t)
	if got := toolImageCount(nil); got != 0 {
		t.Errorf("nil tool call = %d, want 0", got)
	}
	if got := toolImageCount(&model.ToolCall{Name: "Bash"}); got != 0 {
		t.Errorf("no result = %d, want 0", got)
	}
	if got := toolImageCount(&model.ToolCall{Result: &model.ToolResult{Content: "x"}}); got != 0 {
		t.Errorf("imageless result = %d, want 0", got)
	}
	two := &model.ToolCall{Result: &model.ToolResult{Images: []model.Image{img, img}}}
	if got := toolImageCount(two); got != 2 {
		t.Errorf("result images = %d, want 2", got)
	}
	// One result image + a sidechain holding a pasted image block AND a nested
	// tool call with its own result image: 3 in total.
	side := &model.ToolCall{
		Result: &model.ToolResult{Images: []model.Image{img}},
		Subagents: []model.Subagent{{Turns: []model.Turn{{
			Kind: model.TurnUser,
			Blocks: []model.Block{
				{Type: model.BlockImage, Image: &img},
				{Type: model.BlockToolUse, Tool: &model.ToolCall{
					Result: &model.ToolResult{Images: []model.Image{img}},
				}},
			},
		}}}},
	}
	if got := toolImageCount(side); got != 3 {
		t.Errorf("sidechain sum = %d, want 3", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/ -run TestToolImageCount -v`
Expected: compile FAIL — `undefined: toolImageCount`.

- [ ] **Step 3: Implement**

In `internal/render/images.go`, replace the body of `forEachImage`'s inner loop so the per-tool-call part lives in a shared walker, and add the two new functions directly after `forEachImage`:

Replace this part of `forEachImage` (keep the function's doc comment and the surrounding `walkTurns` / `s.Turns` / `s.Agents` structure exactly as is):

```go
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
```

with:

```go
	walkTurns = func(turns []model.Turn) {
		for _, t := range turns {
			for _, blk := range t.Blocks {
				if blk.Type == model.BlockImage && blk.Image != nil {
					fn(*blk.Image)
				}
				if blk.Type == model.BlockToolUse {
					forEachToolCallImage(blk.Tool, fn)
				}
			}
		}
	}
```

Then add after `forEachImage`:

```go
// forEachToolCallImage calls fn for every image a tool call carries: its
// result's images and, recursively, everything inside its nested sidechain
// turns (which can themselves contain tool calls with images).
func forEachToolCallImage(tc *model.ToolCall, fn func(model.Image)) {
	if tc == nil {
		return
	}
	if tc.Result != nil {
		for _, img := range tc.Result.Images {
			fn(img)
		}
	}
	for _, sub := range tc.Subagents {
		for _, t := range sub.Turns {
			for _, blk := range t.Blocks {
				if blk.Type == model.BlockImage && blk.Image != nil {
					fn(*blk.Image)
				}
				if blk.Type == model.BlockToolUse {
					forEachToolCallImage(blk.Tool, fn)
				}
			}
		}
	}
}

// toolImageCount is the number of images a collapsed tool card hides: result
// images plus everything inside nested sidechain turns.
func toolImageCount(tc *model.ToolCall) int {
	n := 0
	forEachToolCallImage(tc, func(model.Image) { n++ })
	return n
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render/ -v`
Expected: PASS — including the pre-existing image tests (`TestSiteWritesImagesAndTags`, `TestSiteSubagentPageImageUsesBasePath`, `TestEndToEndImages`), which pin `forEachImage`'s behavior across the refactor.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add internal/render/images.go internal/render/images_test.go
git commit -m "refactor(render): shared per-tool-call image walker + count

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

### Task 2: Badge markup, header wiring, CSS

**Files:**
- Modify: `internal/render/images.go` (imports; new func at end)
- Modify: `internal/render/render.go` (renderTool ~line 360, between the tool-summary block and the agent-link block)
- Modify: `internal/render/assets/styles.css` (`.agent-link` rule at line 114; append at end)
- Test: `internal/render/images_test.go`

**Interfaces:**
- Consumes: `toolImageCount(tc *model.ToolCall) int` from Task 1.
- Produces: `imageBadge(tc *model.ToolCall) string` (render-internal; nothing downstream consumes it).

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/images_test.go`:

```go
func TestImageBadge(t *testing.T) {
	img := testImage(t)
	if got := imageBadge(&model.ToolCall{}); got != "" {
		t.Errorf("no images: %q, want empty", got)
	}
	one := &model.ToolCall{Result: &model.ToolResult{Images: []model.Image{img}}}
	want := `<span class="image-badge"><span class="image-badge-icon">📷</span></span>`
	if got := imageBadge(one); got != want {
		t.Errorf("one image:\n got %q\nwant %q", got, want)
	}
	three := &model.ToolCall{Result: &model.ToolResult{Images: []model.Image{img, img, img}}}
	want = `<span class="image-badge"><span class="image-badge-icon">📷</span> 3</span>`
	if got := imageBadge(three); got != want {
		t.Errorf("three images:\n got %q\nwant %q", got, want)
	}
}

func TestSiteToolCardShowsImageBadge(t *testing.T) {
	img := testImage(t)
	s := model.Session{Title: "b", Turns: []model.Turn{
		{Kind: model.TurnAssistant, Blocks: []model.Block{
			{Type: model.BlockToolUse, Tool: &model.ToolCall{
				ID: "t1", Name: "Read", Summary: "a.png",
				Result: &model.ToolResult{Images: []model.Image{img, img}},
			}},
			{Type: model.BlockToolUse, Tool: &model.ToolCall{
				ID: "t2", Name: "Bash", Summary: "ls",
				Result: &model.ToolResult{Content: "ok"},
			}},
		}},
	}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	sp := string(page)
	// Two identical images dedupe to one asset file but still count as 2.
	if !strings.Contains(sp, `<span class="image-badge"><span class="image-badge-icon">📷</span> 2</span>`) {
		t.Error("badge with count 2 missing")
	}
	// `class="image-badge">` (with closing quote+bracket) cannot match the
	// icon span's class attribute, so this counts whole badges only.
	if got := strings.Count(sp, `class="image-badge">`); got != 1 {
		t.Errorf("found %d badges, want 1 (the Bash card must not badge)", got)
	}
}

func TestSiteImageBadgeShownWithNoImages(t *testing.T) {
	img := testImage(t)
	s := model.Session{Title: "b", Turns: []model.Turn{
		{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
			ID: "t1", Name: "Read", Summary: "a.png",
			Result: &model.ToolResult{Images: []model.Image{img}},
		}}}},
	}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{NoImages: true}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `class="image-badge">`) {
		t.Error("badge must still show when images are omitted")
	}
}

func TestSiteImageBadgePrecedesAgentLink(t *testing.T) {
	img := testImage(t)
	s := model.Session{
		Title: "root",
		Turns: []model.Turn{{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
			ID: "toolu_1", Name: "Agent", Summary: "run checks",
			Result: &model.ToolResult{Content: "done", Images: []model.Image{img}},
		}}}}},
		Agents: []model.AgentSession{{ID: "a1", ToolUseID: "toolu_1"}},
	}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	want := `📷</span></span><a class="agent-link"`
	if !strings.Contains(string(page), want) {
		t.Errorf("badge must directly precede the transcript link; %q not found", want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestImageBadge|TestSiteToolCardShowsImageBadge|TestSiteImageBadge' -v`
Expected: compile FAIL — `undefined: imageBadge`.

- [ ] **Step 3: Implement**

In `internal/render/images.go`, add `"strconv"` to the imports and append at the end of the file:

```go
// imageBadge is the collapsed-header indicator for a tool card that hides
// images: the camera icon, plus the count when more than one. "" when the
// card holds none. Shown regardless of NoImages — omission placeholders
// inside the card are worth flagging too.
func imageBadge(tc *model.ToolCall) string {
	n := toolImageCount(tc)
	if n == 0 {
		return ""
	}
	badge := `<span class="image-badge"><span class="image-badge-icon">📷</span>`
	if n > 1 {
		badge += " " + strconv.Itoa(n)
	}
	return badge + `</span>`
}
```

In `internal/render/render.go`, in `renderTool`, between the tool-summary block and the agent-link block, insert the badge write, so the head section reads:

```go
	b.WriteString(`<details class="tool"><summary class="tool-head">`)
	b.WriteString(`<span class="tool-name">` + html.EscapeString(tc.Name) + `</span>`)
	if s := headerSummary(tc); s != "" {
		b.WriteString(`<span class="tool-summary">` + html.EscapeString(StripANSI(s)) + `</span>`)
	}
	b.WriteString(imageBadge(tc))
	if href := ctx.links.forToolUse(tc.ID); href != "" {
		b.WriteString(`<a class="agent-link" href="` + html.EscapeString(href) + `">transcript ↗</a>`)
	}
```

In `internal/render/assets/styles.css`, replace line 114

```css
.agent-link { margin-left: 0.5rem; font-size: 0.75rem; text-transform: none; letter-spacing: normal; }
```

with

```css
.agent-link { margin-left: 0.5rem; font-size: 0.75rem; text-transform: none; letter-spacing: normal; flex-shrink: 0; }
```

and append at the end of the file:

```css
.image-badge { color: var(--muted); font-size: .75rem; font-weight: 400; white-space: nowrap; flex-shrink: 0; }
/* Default (Chrome/Blink, Firefox/Gecko): small lift, slightly larger glyph. */
.image-badge-icon { position: relative; top: -0.04em; font-size: 0.85rem; }
/* WebKit only (Safari, DuckDuckGo on macOS) — -webkit-hyphens exists in no
   other engine: bigger lift, stock size. */
@supports (-webkit-hyphens: none) {
  .image-badge-icon { top: -0.09em; font-size: 0.75rem; }
}
```

(Why the flex behavior is safe with long clipped paths: in the `.tool-head`
flex row, `.tool-summary` has `overflow: hidden`, giving it a zero flex
minimum — it is the only element that shrinks and it ellipsizes as today,
while badge and link keep `flex-shrink: 0` and stay fully visible.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add internal/render/images.go internal/render/images_test.go internal/render/render.go internal/render/assets/styles.css
git commit -m "feat(render): image badge on tool-card headers

Cards hiding images show 📷 (with count when >1) after the summary,
pinned against squeeze-out by long paths; emoji baseline tuned per
rendering engine via @supports.

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

## Verification (after all tasks)

- `go test ./...` — full suite green; `gofmt -l internal/ cmd/` — no output.
- Manual smoke: `go run ./cmd/ccwhid --session 41f7e8f4 --out /tmp/ccwhid-badge-check --force`; confirm cards with Playwright screenshots / Read-on-image show `📷` (count where >1) at the header's right edge, long paths still ellipsize with the badge visible, and `--no-images` renders keep the badges.
