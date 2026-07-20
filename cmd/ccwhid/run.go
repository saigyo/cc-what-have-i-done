package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

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
	if opts.includeSubagents {
		agents, err := transcript.LoadAgentSessions(si.FilePath, transcript.Options{
			IncludeSubagents: opts.includeSubagents,
		})
		if err != nil {
			return "", err
		}
		sess.Agents = agents
	}
	if !opts.noRedact {
		home, _ := os.UserHomeDir()
		redact.Session(&sess, redact.Config{HomeDir: home, UserName: redactUserName(opts)})
	}
	if err := render.Site(sess, outDir, render.Options{Title: opts.title, Usage: opts.usage, Version: version, NoImages: opts.noImages}); err != nil {
		return "", err
	}
	return outDir, nil
}

// redactUserName resolves the display name to scrub from the report. An explicit
// --redact-name wins; otherwise it falls back to the OS account's display name
// and then to git's configured user.name. Returns "" when name redaction is
// disabled or no name can be found.
func redactUserName(opts *options) string {
	if opts.noRedactName {
		return ""
	}
	if opts.redactName != "" {
		return opts.redactName
	}
	if u, err := user.Current(); err == nil {
		if name := gecosName(u.Name); name != "" {
			return name
		}
	}
	if out, err := exec.Command("git", "config", "--get", "user.name").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name
		}
	}
	return ""
}

// gecosName cleans a display name from os/user, whose Name comes from the Unix
// GECOS field and can carry trailing empty comma-separated sub-fields
// ("Jane Doe,,,"). It drops those trailing empties (keeping a genuine embedded
// field like "Doe, John") and trims surrounding space.
func gecosName(name string) string {
	fields := strings.Split(name, ",")
	for len(fields) > 1 && strings.TrimSpace(fields[len(fields)-1]) == "" {
		fields = fields[:len(fields)-1]
	}
	return strings.TrimSpace(strings.Join(fields, ","))
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

// openInBrowser opens the report's index.html inside dir in the default browser.
func openInBrowser(dir string) error {
	index := filepath.Join(dir, "index.html")
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
