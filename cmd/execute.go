// Package cmd builds the ferry command line: one root command that turns a summary into an
// analogy.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

// Execute runs ferry with the process arguments and exits with the matching code.
func Execute() {
	os.Exit(execute())
}

// execute runs the root command under an interrupt-aware context and returns the exit code.
func execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// The first Ctrl-C cancels the run. Restoring the default handler afterward lets a second
	// Ctrl-C end the process at once if anything is slow to unwind.
	go func() {
		<-ctx.Done()
		stop()
	}()
	err := rootCmd(newClaudeCompleter, isTerminal).ExecuteContext(ctx)
	return report(os.Stderr, err, messageWidth(isTerminal(os.Stderr), os.Stderr))
}

// report prints err to w as "ferry: <message>" unless it is a quiet exit, and returns the exit
// code for it. The message is wrapped to cols when cols is above 0.
func report(w io.Writer, err error, cols int) int {
	var quiet *QuietExitError
	if err != nil && !errors.As(err, &quiet) {
		fmt.Fprintln(w, message("ferry: ", err.Error(), cols))
	}
	return exitCode(err)
}
