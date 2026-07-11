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
	for _, g := range groups {
		for _, s := range g.Sessions {
			if !s.IsAgent {
				return s, false, nil
			}
		}
	}
	return discovery.SessionInfo{}, false, fmt.Errorf("no sessions found")
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
	if len(entries) > 0 && !force {
		return fmt.Errorf("output directory %q is not empty (use --force to overwrite)", dir)
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
	url := "file://" + abs
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
