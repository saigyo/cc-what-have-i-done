package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
	"github.com/saigyo/cc-what-have-i-done/internal/redact"
	"github.com/saigyo/cc-what-have-i-done/internal/render"
	"github.com/saigyo/cc-what-have-i-done/internal/transcript"
)

// errNeedTUI signals that no session selector was provided and the caller
// should launch the interactive browser.
var errNeedTUI = errors.New("no session selected: launch TUI")

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

func latestSession(opts *options, root string) (discovery.SessionInfo, bool, error) {
	groups, err := discovery.Scan(root)
	if err != nil {
		return discovery.SessionInfo{}, false, err
	}
	for _, g := range groups {
		if opts.project != "" && !matchProject(g, opts.project) {
			continue
		}
		if len(g.Sessions) > 0 {
			return g.Sessions[0], false, nil
		}
	}
	return discovery.SessionInfo{}, false, fmt.Errorf("no sessions found")
}

func matchProject(g discovery.ProjectGroup, want string) bool {
	return g.ProjectPath == want ||
		strings.EqualFold(filepath.Base(g.ProjectPath), want) ||
		strings.Contains(g.ProjectPath, want)
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
