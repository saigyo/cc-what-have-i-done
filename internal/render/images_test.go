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
