package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
	"github.com/saigyo/cc-what-have-i-done/internal/redact"
	"github.com/saigyo/cc-what-have-i-done/internal/render"
	"github.com/saigyo/cc-what-have-i-done/internal/transcript"
)

func resolveOutDir(opts *options, si discovery.SessionInfo) string {
	if opts.out != "" {
		return opts.out
	}
	short := si.ID
	if len(short) > 8 {
		short = short[:8]
	}
	return filepath.Join("ccwhid-report", short)
}

// resolveSession picks a session from flags. It returns needTUI=true when the
// user gave no selector.
func resolveSession(opts *options, root string) (discovery.SessionInfo, bool, error) {
	switch {
	case opts.session != "":
		si, err := discovery.FindSession(root, opts.session)
		return si, false, err
	case opts.latest:
		return latestSession(opts, root)
	default:
		return discovery.SessionInfo{}, true, nil
	}
}

// latestSession returns the most recent interactive (non-agent) session,
// optionally scoped to a project. Agent-spawned transcripts are skipped so
// --latest targets the user's own sessions.
func latestSession(opts *options, root string) (discovery.SessionInfo, bool, error) {
	groups, err := discovery.Scan(root)
	if err != nil {
		return discovery.SessionInfo{}, false, err
	}
	if opts.project != "" {
		idx, err := discovery.FindProject(groups, opts.project)
		if err != nil {
			return discovery.SessionInfo{}, false, err
		}
		groups = groups[idx : idx+1]
	}
	// Pick the newest interactive session by modification time. Group order
	// alone is insufficient: groups are sorted by their newest session
	// *including* agent transcripts, so a recent agent run in one project
	// could otherwise mask a newer human session in another.
	bestGI, bestSI := -1, -1
	for gi := range groups {
		for si := range groups[gi].Sessions {
			if groups[gi].Sessions[si].IsAgent {
				continue
			}
			if bestGI < 0 || groups[gi].Sessions[si].ModTime.After(groups[bestGI].Sessions[bestSI].ModTime) {
				bestGI, bestSI = gi, si
			}
		}
	}
	if bestGI < 0 {
		return discovery.SessionInfo{}, false, fmt.Errorf("no sessions found")
	}
	return groups[bestGI].Sessions[bestSI], false, nil
}

// generate parses, redacts, and renders a session, returning the output dir.
func generate(opts *options, si discovery.SessionInfo) (string, error) {
	outDir := resolveOutDir(opts, si)
	if err := ensureOutDir(outDir, opts.force); err != nil {
		return "", err
	}
	sess, err := transcript.ParseFile(si.FilePath, transcript.Options{
		IncludeSubagents: opts.includeSubagents,
	})
	if err != nil {
		return "", err
	}
	if si.ID != "" {
		sess.ID = si.ID
	}
	if sess.SkippedLines > 0 {
		fmt.Fprintf(os.Stderr, "warning: skipped %d malformed line(s)\n", sess.SkippedLines)
	}
	if !opts.noRedact {
		home, _ := os.UserHomeDir()
		redact.Session(&sess, home)
	}
	if err := render.Site(sess, outDir, render.Options{Title: opts.title}); err != nil {
		return "", err
	}
	return outDir, nil
}

func ensureOutDir(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if !force {
		return fmt.Errorf("output directory %q is not empty (use --force to overwrite)", dir)
	}
	// --force: clear existing entries so the report reflects only this run and
	// no stale files (e.g. renamed assets) are left behind.
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// openInBrowser opens path (a file or dir) in the default browser.
func openInBrowser(path string) error {
	index := filepath.Join(path, "index.html")
	abs, err := filepath.Abs(index)
	if err != nil {
		return err
	}
	// Pass the filesystem path directly rather than a file:// URL: exec runs the
	// opener without a shell, so a single path argument handles spaces and
	// special characters, and avoids Windows backslash/URL-encoding pitfalls.
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", abs).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}
