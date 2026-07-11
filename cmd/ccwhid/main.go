package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
	"github.com/saigyo/cc-what-have-i-done/internal/tui"
)

// options holds all CLI flag values.
type options struct {
	session          string
	project          string
	latest           bool
	out              string
	title            string
	includeSubagents bool
	noRedact         bool
	force            bool
	open             bool
}

func newRootCmd() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "ccwhid",
		Short: "Turn a Claude Code session transcript into a browsable HTML report",
		Long: "ccwhid (cc-what-have-i-done) renders a Claude Code session " +
			"transcript into a self-contained static HTML report. Run with no " +
			"flags to browse sessions in an interactive TUI.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.session, "session", "", "session id or unambiguous prefix to render")
	f.StringVar(&opts.project, "project", "", "project path or name to scope to")
	f.BoolVar(&opts.latest, "latest", false, "render the most recent session")
	f.StringVar(&opts.out, "out", "", "output directory (default ./ccwhid-report/<session-short>)")
	f.StringVar(&opts.title, "title", "", "override the report title")
	f.BoolVar(&opts.includeSubagents, "include-subagents", true, "include subagent (Task) activity")
	f.BoolVar(&opts.noRedact, "no-redact", false, "disable secret redaction")
	f.BoolVar(&opts.force, "force", false, "overwrite a non-empty output directory")
	f.BoolVar(&opts.open, "open", false, "open the report in a browser when done")
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
		sel, err := tui.Run(groups)
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
