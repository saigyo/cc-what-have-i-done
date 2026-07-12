package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
	"github.com/saigyo/cc-what-have-i-done/internal/tui"
)

// version is the build version, overridden at release time via
// -ldflags "-X main.version=…" (GoReleaser). Defaults to "dev" for local builds.
var version = "dev"

// options holds all CLI flag values.
type options struct {
	session          string
	project          string
	latest           bool
	out              string
	title            string
	includeSubagents bool
	noRedact         bool
	redactName       string
	noRedactName     bool
	force            bool
	open             bool
	usage            bool
}

func newRootCmd() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "ccwhid",
		Short: "Turn a Claude Code session transcript into a browsable HTML report",
		Long: "ccwhid (cc-what-have-i-done) renders a Claude Code session " +
			"transcript into a self-contained static HTML report. Run with no " +
			"flags to browse sessions in an interactive TUI.",
		Version:      version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.session, "session", "", "session id or unambiguous prefix to render")
	f.StringVar(&opts.project, "project", "", "project path or name; scopes --latest, or opens the TUI focused on it")
	f.BoolVar(&opts.latest, "latest", false, "render the most recent interactive session (skips agent transcripts)")
	f.StringVar(&opts.out, "out", "", "output directory (default ./ccwhid-report/<session-short>)")
	f.StringVar(&opts.title, "title", "", "override the report title")
	f.BoolVar(&opts.includeSubagents, "include-subagents", true, "include subagent work: inline Task sidechains and linked agent-session pages")
	f.BoolVar(&opts.noRedact, "no-redact", false, "disable secret redaction")
	f.StringVar(&opts.redactName, "redact-name", "", "display name to scrub from output (default: your OS/git display name)")
	f.BoolVar(&opts.noRedactName, "no-redact-name", false, "disable display-name redaction (account/path redaction still applies)")
	f.BoolVar(&opts.force, "force", false, "overwrite a non-empty output directory")
	f.BoolVar(&opts.open, "open", false, "open the report in a browser when done")
	f.BoolVar(&opts.usage, "usage", false, "include a token-usage & estimated-cost section")
	return cmd
}

func run(cmd *cobra.Command, opts *options) error {
	root, err := discovery.DefaultRoot()
	if err != nil {
		return err
	}
	si, needTUI, err := resolveSession(opts, root)
	if err != nil {
		return err
	}
	if needTUI {
		groups, err := discovery.Scan(root)
		if err != nil {
			return err
		}
		focusIdx := -1
		if opts.project != "" {
			focusIdx, err = discovery.FindProject(groups, opts.project)
			if err != nil {
				return err
			}
		}
		sel, err := tui.Run(groups, focusIdx)
		if err != nil {
			return err
		}
		if sel.Canceled {
			fmt.Fprintln(cmd.OutOrStdout(), "canceled")
			return nil
		}
		si = sel.Session
		opts.includeSubagents = sel.IncludeSubagents
		opts.noRedact = !sel.Redact
		opts.open = sel.Open
		opts.usage = sel.Usage
		if sel.OutDir != "" {
			opts.out = sel.OutDir
		}
	}
	outDir, err := generate(opts, si)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "report written to %s\n", outDir)
	if opts.open {
		if err := openInBrowser(outDir); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not open browser: %v\n", err)
		}
	}
	return nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
