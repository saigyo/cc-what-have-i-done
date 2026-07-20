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
