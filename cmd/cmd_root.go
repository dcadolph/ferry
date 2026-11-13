package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dcadolph/ferry/cmd/config"
	"github.com/dcadolph/ferry/internal/claude"
	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/jsonutil"
	"github.com/dcadolph/ferry/internal/logutil"
	"github.com/dcadolph/ferry/internal/render"
	"github.com/dcadolph/ferry/internal/util"
)

// longHelp is the root command's help text.
const longHelp = `ferry turns a plain summary of a messy situation into an analogy you can say out
loud, such as "we keep topping off the oil instead of fixing the leak."

Give the summary as arguments, with --file, or on stdin. Quote it so the shell
keeps it in one piece. Every argument is summary text, so ferry has no
subcommands. ferry pulls out the structure of the situation, drafts analogies
from several familiar worlds, checks each one against that structure, and prints
the one that holds up, with what stands for what and where the comparison
breaks.

Your summary is sent to Claude. Remove names, company names, and other specifics
from day-job material first. ferry does not scrub anything itself.

Every flag can also be set in the environment as FERRY_<FLAG>, e.g.
FERRY_AUDIENCE or FERRY_LOG_LEVEL. A flag on the command line wins over the
environment.`

// completerFactory builds the model client for one run from the client options. Tests pass a
// factory that returns a mock, so a full run needs no network.
type completerFactory func(opts claude.Options) (engine.Completer, error)

// newClaudeCompleter is the production completerFactory. It returns a Claude client.
func newClaudeCompleter(opts claude.Options) (engine.Completer, error) {
	c, err := claude.New(opts)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// terminalFunc reports whether v, one of the command's streams, is an interactive terminal.
// Tests pass their own, so a run can act as if it had one.
type terminalFunc func(v any) bool

// rootCmd builds the ferry root command. It has no subcommands, so no word of a summary can be
// read as a command, and it prints the version with --version. newCompleter builds the model
// client for each run, and terminal decides whether a stream gets prompts and progress lines.
func rootCmd(newCompleter completerFactory, terminal terminalFunc) *cobra.Command {
	if newCompleter == nil {
		panic("rootCmd: completer factory required")
	}
	if terminal == nil {
		panic("rootCmd: terminal function required")
	}
	cmd := &cobra.Command{
		Use:           "ferry [flags] [summary...]",
		Short:         "Turn a messy situation into an analogy you can say out loud.",
		Long:          longHelp,
		Version:       resolveVersion(),
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoot(cmd, args, newCompleter, terminal)
		},
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	})
	f := cmd.Flags()
	f.AddFlag(&config.FlagAudience)
	f.AddFlag(&config.FlagWorld)
	f.AddFlag(&config.FlagModel)
	f.AddFlag(&config.FlagEffort)
	f.AddFlag(&config.FlagFile)
	f.AddFlag(&config.FlagJSON)
	f.AddFlag(&config.FlagPretty)
	f.AddFlag(&config.FlagShowWork)
	f.AddFlag(&config.FlagTimeout)
	f.AddFlag(&config.FlagLogLevel)
	return cmd
}

// runRoot does one ferry run: it checks the settings, prints the privacy reminder, reads the
// summary, runs the engine, and prints the result. The summary is never read, and so never
// sent, unless the reminder was written first.
func runRoot(cmd *cobra.Command, args []string, newCompleter completerFactory, terminal terminalFunc) error {
	ctx := cmd.Context()
	stdin, stdout, stderr := cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	s, err := loadSettings()
	if err != nil {
		return err
	}
	errTerm := terminal(stderr)
	errStyle := render.NewStyle(render.ColorWanted(errTerm))
	src := source{
		args:     args,
		file:     s.file,
		stdin:    stdin,
		terminal: terminal(stdin),
		prompt:   stderr,
		style:    errStyle,
	}
	if err := src.check(); err != nil {
		return err
	}
	logger, err := logutil.New(stderr, s.logLevel)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}
	// Syncing stderr fails on some terminals with a harmless error, so the result is dropped.
	defer func() { _ = logger.Sync() }()
	ctx = logutil.WrapLogger(ctx, logger)

	if err := printReminder(stderr, errStyle); err != nil {
		return fmt.Errorf("%w: privacy reminder: %w", ErrWrite, err)
	}
	summary, err := src.read(ctx)
	if err != nil {
		return interrupted(ctx, err)
	}
	completer, err := newCompleter(s.clientOptions())
	if err != nil {
		return fmt.Errorf("%w: %w", ErrClient, err)
	}
	cfg := engine.DefaultConfig()
	prog := progressFunc(stderr, display{
		terminal: errTerm,
		live:     errTerm && canRedraw(stderr),
		style:    errStyle,
		width:    streamWidth(stderr),
	}, logger)
	cfg.Progress = prog.report
	start := time.Now()
	logger.Debug("run started",
		zap.String("model", s.model),
		zap.String("effort", s.effort),
		zap.Duration("timeout", s.timeout),
		zap.Bool("audience_set", s.audience != ""),
		zap.Int("worlds", len(s.worlds)),
		zap.Int("summary_bytes", len(summary)),
	)
	res, err := engine.NewWithConfig(completer, cfg).Run(ctx, s.input(summary))
	prog.finish(interrupted(ctx, err))
	out := output{
		style: render.NewStyle(!s.asJSON && render.ColorWanted(terminal(stdout))),
		cols:  render.Width(streamWidth(stdout)),
	}
	if err != nil {
		return runFailed(ctx, stdout, res, s, out, err)
	}
	logger.Debug("run finished",
		zap.Int("rounds", res.Rounds),
		zap.Int("candidates", len(res.Checked)),
		zap.String("world", res.Analogy.World),
		zap.Bool("note", res.Note != ""),
		zap.Duration("elapsed", time.Since(start)),
	)
	printNote(stderr, res.Note, messageWidth(errTerm, stderr))
	return writeResult(stdout, res, s, out)
}

// output holds how text on stdout is styled and how wide it is laid out.
type output struct {
	// style colors the text.
	style render.Style
	// cols is the width the text is wrapped to.
	cols int
}

// printNote writes the engine's note about the result to w as a "ferry: note:" message, when
// there is one, wrapped to cols when cols is above 0. It goes to stderr on every run, terminal
// or not, so a result that is not a clean polished analogy never passes without a word.
func printNote(w io.Writer, note string, cols int) {
	if note = util.CollapseSpace(note); note != "" {
		fmt.Fprintln(w, message("ferry: note: ", note, cols))
	}
}

// message returns text after prefix, wrapped to cols under its own first line when cols is
// above 0, and on one line otherwise.
func message(prefix, text string, cols int) string {
	if cols < 1 {
		return prefix + text
	}
	return render.Hang(prefix, text, cols)
}

// messageWidth returns the width a message on w is wrapped to: the clamped terminal width when
// w is a terminal, and 0, meaning one line, when it is not.
func messageWidth(isTerm bool, w any) int {
	if !isTerm {
		return 0
	}
	return render.Width(streamWidth(w))
}

// interrupted turns err into a quiet exit when ctx ended because the user pressed Ctrl-C.
func interrupted(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return &QuietExitError{Code: codeInterrupted}
	}
	return err
}

// runFailed returns the error for an engine run that ended in err. When no analogy held up, it
// first prints the work that was paid for to w, with or without --show-work: the skeleton and
// every candidate as text, or the partial result as JSON. The user sees why each candidate was
// dropped without running again.
func runFailed(ctx context.Context, w io.Writer, res engine.Result, s settings, out output, err error) error {
	err = interrupted(ctx, err)
	if !errors.Is(err, engine.ErrNoFit) {
		return err
	}
	var werr error
	if s.asJSON {
		werr = writeJSON(w, res, s.pretty)
	} else {
		werr = render.Work(w, res, out.style, out.cols)
	}
	if werr != nil {
		return errors.Join(err, werr)
	}
	return err
}

// writeResult prints res to w as JSON or as text, as the settings ask. Text is laid out as out
// says.
func writeResult(w io.Writer, res engine.Result, s settings, out output) error {
	if s.asJSON {
		return writeJSON(w, res, s.pretty)
	}
	return render.Text(w, res, s.showWork, out.style, out.cols)
}

// writeJSON prints res to w as one JSON value and a newline, indented when pretty is set.
func writeJSON(w io.Writer, res engine.Result, pretty bool) error {
	b, err := jsonutil.Marshal(res, pretty)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, string(b)); err != nil {
		return fmt.Errorf("%w: %w", ErrWrite, err)
	}
	return nil
}
