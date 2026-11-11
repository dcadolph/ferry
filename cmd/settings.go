package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/dcadolph/ferry/cmd/config"
	"github.com/dcadolph/ferry/internal/claude"
	"github.com/dcadolph/ferry/internal/engine"
)

// settings holds the resolved configuration for one run.
type settings struct {
	// audience describes who will hear the analogy, or is empty for the engine default.
	audience string
	// worlds lists the preferred worlds, most preferred first.
	worlds []string
	// model is the Claude model ID.
	model string
	// effort is the reasoning effort.
	effort string
	// file is the --file path, "-" for stdin, or empty.
	file string
	// asJSON prints the result as JSON.
	asJSON bool
	// pretty indents the JSON output.
	pretty bool
	// showWork adds the skeleton and every candidate to text output.
	showWork bool
	// timeout caps each model call, retries included.
	timeout time.Duration
	// logLevel is the lowest log level written to stderr.
	logLevel string
}

// loadSettings resolves every flag and rejects values ferry cannot run with. Every rejection
// is a usage error, whether the value came from the command line or the environment.
func loadSettings() (settings, error) {
	if err := config.CheckEnv(); err != nil {
		return settings{}, fmt.Errorf("%w: %w", ErrUsage, err)
	}
	s := settings{
		audience: strings.TrimSpace(config.Audience()),
		worlds:   config.World(),
		model:    strings.TrimSpace(config.Model()),
		effort:   strings.ToLower(strings.TrimSpace(config.Effort())),
		file:     config.File(),
		asJSON:   config.JSON(),
		pretty:   config.Pretty(),
		showWork: config.ShowWork(),
		timeout:  config.Timeout(),
		logLevel: config.LogLevel(),
	}
	if s.model == "" {
		return settings{}, fmt.Errorf("%w: --model is empty", ErrUsage)
	}
	if err := claude.CheckEffort(s.effort); err != nil {
		return settings{}, fmt.Errorf("%w: %w", ErrUsage, err)
	}
	distinct := len(engine.DistinctWorlds(s.worlds))
	if limit := engine.DefaultConfig().MaxWorlds; distinct > limit {
		return settings{}, fmt.Errorf("%w: --world takes at most %d worlds, got %d",
			ErrUsage, limit, distinct)
	}
	if s.timeout <= 0 {
		return settings{}, fmt.Errorf("%w: --timeout must be above zero, got %s",
			ErrUsage, s.timeout)
	}
	return s, nil
}

// clientOptions returns the Claude client options for the settings. The reply cap follows the
// effort, since thinking counts against it.
func (s settings) clientOptions() claude.Options {
	return claude.Options{
		Model:     s.model,
		Effort:    s.effort,
		MaxTokens: claude.MaxTokensFor(s.effort),
		Timeout:   s.timeout,
	}
}

// input returns the engine input for summary under the settings.
func (s settings) input(summary string) engine.Input {
	return engine.Input{Summary: summary, Audience: s.audience, Worlds: s.worlds}
}
