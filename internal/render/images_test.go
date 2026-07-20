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
