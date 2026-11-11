package cmd

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/pflag"

	"github.com/dcadolph/ferry/cmd/config"
	"github.com/dcadolph/ferry/internal/claude"
	"github.com/dcadolph/ferry/internal/engine"
)

// TestLoadSettings checks resolution from flags and the environment, the clean-up of each value,
// and the values ferry refuses. It runs serially because flag state is package-global and
// t.Setenv forbids t.Parallel.
func TestLoadSettings(t *testing.T) {
	defaults := settings{
		model:    config.DefaultModel,
		effort:   config.DefaultEffort,
		timeout:  config.DefaultTimeout,
		logLevel: config.DefaultLogLevel,
	}
	tests := []struct {
		Want       error
		Env        map[string]string
		Flags      map[*pflag.Flag]string
		WantResult settings
	}{{ // Test 0: Nothing set gives the defaults.
		WantResult: defaults, Want: nil,
	}, { // Test 1: Every flag on the command line, with text values cleaned up.
		Flags: map[*pflag.Flag]string{
			&config.FlagAudience: "  the board  ",
			&config.FlagWorld:    "cars, sports",
			&config.FlagModel:    " claude-test ",
			&config.FlagEffort:   " MAX ",
			&config.FlagFile:     "notes.txt",
			&config.FlagJSON:     "true",
			&config.FlagPretty:   "true",
			&config.FlagShowWork: "true",
			&config.FlagTimeout:  "30s",
			&config.FlagLogLevel: "debug",
		},
		WantResult: settings{
			audience: "the board",
			worlds:   []string{"cars", "sports"},
			model:    "claude-test",
			effort:   "max",
			file:     "notes.txt",
			asJSON:   true,
			pretty:   true,
			showWork: true,
			timeout:  30 * time.Second,
			logLevel: "debug",
		},
		Want: nil,
	}, { // Test 2: The environment sets values the command line leaves alone.
		Env:   map[string]string{"FERRY_EFFORT": "low", "FERRY_WORLD": "boats,trains"},
		Flags: map[*pflag.Flag]string{&config.FlagEffort: "medium"},
		WantResult: settings{
			worlds:   []string{"boats", "trains"},
			model:    config.DefaultModel,
			effort:   "medium",
			timeout:  config.DefaultTimeout,
			logLevel: config.DefaultLogLevel,
		},
		Want: nil,
	}, { // Test 3: An unknown effort from the environment is a usage error.
		Env:  map[string]string{"FERRY_EFFORT": "extreme"},
		Want: claude.ErrEffort,
	}, { // Test 4: A zero timeout is a usage error.
		Flags: map[*pflag.Flag]string{&config.FlagTimeout: "0s"},
		Want:  ErrUsage,
	}, { // Test 5: A negative timeout is a usage error.
		Flags: map[*pflag.Flag]string{&config.FlagTimeout: "-1m"},
		Want:  ErrUsage,
	}, { // Test 6: A blank model is a usage error.
		Flags: map[*pflag.Flag]string{&config.FlagModel: "  "},
		Want:  ErrUsage,
	}, { // Test 7: A bool that does not parse in the environment is a usage error.
		Env:  map[string]string{"FERRY_JSON": "yes"},
		Want: config.ErrValue,
	}, { // Test 8: A duration that does not parse in the environment is a usage error.
		Env:  map[string]string{"FERRY_TIMEOUT": "soon"},
		Want: config.ErrValue,
	}, { // Test 9: A duration with no unit in the environment is a usage error.
		Env:  map[string]string{"FERRY_TIMEOUT": "90"},
		Want: config.ErrValue,
	}, { // Test 10: A flag on the command line wins over a bad value in the environment.
		Env:   map[string]string{"FERRY_TIMEOUT": "soon", "FERRY_PRETTY": "maybe"},
		Flags: map[*pflag.Flag]string{&config.FlagTimeout: "45s", &config.FlagPretty: "true"},
		WantResult: settings{
			model:    config.DefaultModel,
			effort:   config.DefaultEffort,
			pretty:   true,
			timeout:  45 * time.Second,
			logLevel: config.DefaultLogLevel,
		},
		Want: nil,
	}, { // Test 11: Good bools and durations in the environment are used.
		Env: map[string]string{
			"FERRY_JSON": "true", "FERRY_SHOW_WORK": "1", "FERRY_TIMEOUT": "90s",
		},
		WantResult: settings{
			model:    config.DefaultModel,
			effort:   config.DefaultEffort,
			asJSON:   true,
			showWork: true,
			timeout:  90 * time.Second,
			logLevel: config.DefaultLogLevel,
		},
		Want: nil,
	}, { // Test 12: As many preferred worlds as one round drafts are allowed.
		Flags: map[*pflag.Flag]string{&config.FlagWorld: "a,b,c,d,e,f"},
		WantResult: settings{
			worlds:   []string{"a", "b", "c", "d", "e", "f"},
			model:    config.DefaultModel,
			effort:   config.DefaultEffort,
			timeout:  config.DefaultTimeout,
			logLevel: config.DefaultLogLevel,
		},
		Want: nil,
	}, { // Test 13: One preferred world more than a round drafts is a usage error.
		Flags: map[*pflag.Flag]string{&config.FlagWorld: "a,b,c,d,e,f,g"},
		Want:  ErrUsage,
	}, { // Test 14: Too many preferred worlds from the environment is a usage error.
		Env:  map[string]string{"FERRY_WORLD": "a,b,c,d,e,f,g"},
		Want: ErrUsage,
	}, { // Test 15: Repeats of one world count once toward the limit.
		Flags: map[*pflag.Flag]string{&config.FlagWorld: "cars,Cars, cars ,cars,cars,cars,sports"},
		WantResult: settings{
			worlds:   []string{"cars", "Cars", "cars", "cars", "cars", "cars", "sports"},
			model:    config.DefaultModel,
			effort:   config.DefaultEffort,
			timeout:  config.DefaultTimeout,
			logLevel: config.DefaultLogLevel,
		},
		Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			config.Reset()
			t.Cleanup(config.Reset)
			clearEnv(t)
			for name, value := range test.Env {
				t.Setenv(name, value)
			}
			for f, raw := range test.Flags {
				setTestFlag(t, f, raw)
			}
			got, err := loadSettings()
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if err != nil && !errors.Is(err, ErrUsage) {
				t.Errorf("error = %v, want it to be a usage error", err)
			}
			opts := []cmp.Option{cmp.AllowUnexported(settings{}), cmpopts.EquateEmpty()}
			if diff := cmp.Diff(test.WantResult, got, opts...); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSettingsMapping checks how settings become client options and engine input.
func TestSettingsMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         settings
		Summary    string
		WantOpts   claude.Options
		WantResult engine.Input
	}{{ // Test 0: Every field that matters reaches the client and the engine.
		In: settings{
			audience: "the board",
			worlds:   []string{"sports"},
			model:    "claude-test",
			effort:   "max",
			timeout:  30 * time.Second,
		},
		Summary: "the summary",
		WantOpts: claude.Options{
			Model:     "claude-test",
			Effort:    "max",
			MaxTokens: claude.DeepMaxTokens,
			Timeout:   30 * time.Second,
		},
		WantResult: engine.Input{
			Summary: "the summary", Audience: "the board", Worlds: []string{"sports"},
		},
	}, { // Test 1: An empty audience and no worlds leave the engine its defaults.
		In:      settings{model: "claude-test", effort: "low", timeout: time.Minute},
		Summary: "the summary",
		WantOpts: claude.Options{
			Model:     "claude-test",
			Effort:    "low",
			MaxTokens: claude.DefaultMaxTokens,
			Timeout:   time.Minute,
		},
		WantResult: engine.Input{Summary: "the summary"},
	}, { // Test 2: The xhigh effort gets the larger reply cap too.
		In:      settings{model: "claude-test", effort: "xhigh", timeout: time.Minute},
		Summary: "the summary",
		WantOpts: claude.Options{
			Model:     "claude-test",
			Effort:    "xhigh",
			MaxTokens: claude.DeepMaxTokens,
			Timeout:   time.Minute,
		},
		WantResult: engine.Input{Summary: "the summary"},
	}, { // Test 3: The default effort keeps the default reply cap.
		In:      settings{model: "claude-test", effort: "high", timeout: time.Minute},
		Summary: "the summary",
		WantOpts: claude.Options{
			Model:     "claude-test",
			Effort:    "high",
			MaxTokens: claude.DefaultMaxTokens,
			Timeout:   time.Minute,
		},
		WantResult: engine.Input{Summary: "the summary"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantOpts, test.In.clientOptions()); diff != "" {
				t.Errorf("options mismatch (-want +got):\n%s", diff)
			}
			got := test.In.input(test.Summary)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("input mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
