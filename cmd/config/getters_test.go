package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/pflag"
)

// Tests in this file run serially because flag state is package-global and t.Setenv forbids
// t.Parallel.

// clearEnv unsets every FERRY_ variable for the rest of the test, so a developer's shell
// cannot leak into a getter's result.
func clearEnv(t *testing.T) {
	t.Helper()
	for key := range flags {
		name := envKey(key)
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
}

// setFlag sets f to raw as if it came from the command line.
func setFlag(t *testing.T, f *pflag.Flag, raw string) {
	t.Helper()
	if err := f.Value.Set(raw); err != nil {
		t.Fatalf("set --%s %q: %v", f.Name, raw, err)
	}
	f.Changed = true
}

// TestStringGetters checks each string getter: the default, an environment value, and a
// command-line value that beats the environment.
func TestStringGetters(t *testing.T) {
	tests := []struct {
		Get        func() string
		Flag       *pflag.Flag
		Env        string
		WantResult string
	}{{ // Test 0: Audience defaults to empty.
		Get: Audience, Flag: &FlagAudience, Env: "FERRY_AUDIENCE", WantResult: "",
	}, { // Test 1: Model defaults to the Claude client default.
		Get: Model, Flag: &FlagModel, Env: "FERRY_MODEL", WantResult: DefaultModel,
	}, { // Test 2: Effort defaults to the Claude client default.
		Get: Effort, Flag: &FlagEffort, Env: "FERRY_EFFORT", WantResult: DefaultEffort,
	}, { // Test 3: File defaults to empty.
		Get: File, Flag: &FlagFile, Env: "FERRY_FILE", WantResult: "",
	}, { // Test 4: The log level defaults to warn.
		Get: LogLevel, Flag: &FlagLogLevel, Env: "FERRY_LOG_LEVEL", WantResult: "warn",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			Reset()
			t.Cleanup(Reset)
			clearEnv(t)
			if diff := cmp.Diff(test.WantResult, test.Get()); diff != "" {
				t.Errorf("default mismatch (-want +got):\n%s", diff)
			}
			t.Setenv(test.Env, "from-env")
			if diff := cmp.Diff("from-env", test.Get()); diff != "" {
				t.Errorf("environment mismatch (-want +got):\n%s", diff)
			}
			setFlag(t, test.Flag, "from-flag")
			if diff := cmp.Diff("from-flag", test.Get()); diff != "" {
				t.Errorf("flag mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestBoolGetters checks each bool getter: the default, an environment value, junk that
// CheckEnv rejects, and a command-line value that beats the environment.
func TestBoolGetters(t *testing.T) {
	tests := []struct {
		Get  func() bool
		Flag *pflag.Flag
		Env  string
	}{{ // Test 0: JSON.
		Get: JSON, Flag: &FlagJSON, Env: "FERRY_JSON",
	}, { // Test 1: Pretty.
		Get: Pretty, Flag: &FlagPretty, Env: "FERRY_PRETTY",
	}, { // Test 2: Show work.
		Get: ShowWork, Flag: &FlagShowWork, Env: "FERRY_SHOW_WORK",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			Reset()
			t.Cleanup(Reset)
			clearEnv(t)
			if test.Get() {
				t.Error("default = true, want false")
			}
			t.Setenv(test.Env, "true")
			if !test.Get() {
				t.Error("with environment true = false, want true")
			}
			if err := CheckEnv(); err != nil {
				t.Errorf("CheckEnv with environment true = %v, want nil", err)
			}
			t.Setenv(test.Env, "not-a-bool")
			err := CheckEnv()
			if !errors.Is(err, ErrValue) {
				t.Fatalf("CheckEnv with junk environment = %v, want %v", err, ErrValue)
			}
			if !strings.Contains(err.Error(), test.Env) {
				t.Errorf("CheckEnv error %q does not name %s", err, test.Env)
			}
			setFlag(t, test.Flag, "false")
			if err := CheckEnv(); err != nil {
				t.Errorf("CheckEnv with a flag over junk environment = %v, want nil", err)
			}
			t.Setenv(test.Env, "true")
			if test.Get() {
				t.Error("with flag false over environment true = true, want false")
			}
		})
	}
}

// TestTimeout checks the duration getter with CheckEnv in front of it, as the command calls
// them: the default, an environment value, junk that CheckEnv rejects, and a command-line
// value that beats the environment.
func TestTimeout(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	clearEnv(t)
	tests := []struct {
		Want       error
		Env        string
		Flag       string
		WantResult time.Duration
	}{{ // Test 0: Nothing set gives five minutes.
		WantResult: 5 * time.Minute,
	}, { // Test 1: The environment sets it.
		Env: "90s", WantResult: 90 * time.Second,
	}, { // Test 2: Junk in the environment is rejected, never swapped for the default.
		Env: "soon", Want: ErrValue,
	}, { // Test 3: The flag beats the environment.
		Env: "90s", Flag: "2m", WantResult: 2 * time.Minute,
	}, { // Test 4: A number with no unit in the environment is rejected.
		Env: "90", Want: ErrValue,
	}, { // Test 5: The flag beats junk in the environment.
		Env: "soon", Flag: "2m", WantResult: 2 * time.Minute,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			Reset()
			clearEnv(t)
			if test.Env != "" {
				t.Setenv("FERRY_TIMEOUT", test.Env)
			}
			if test.Flag != "" {
				setFlag(t, &FlagTimeout, test.Flag)
			}
			err := CheckEnv()
			if !errors.Is(err, test.Want) {
				t.Fatalf("CheckEnv() = %v, want %v", err, test.Want)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "FERRY_TIMEOUT") {
					t.Errorf("CheckEnv error %q does not name FERRY_TIMEOUT", err)
				}
				return
			}
			if diff := cmp.Diff(test.WantResult, Timeout()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckEnv checks which environment values CheckEnv rejects: bools and durations that do
// not parse, and nothing else.
func TestCheckEnv(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	clearEnv(t)
	tests := []struct {
		Want     error
		Env      map[string]string
		WantName string
	}{{ // Test 0: Nothing set.
		Want: nil,
	}, { // Test 1: Good values of every type.
		Env: map[string]string{
			"FERRY_JSON": "true", "FERRY_PRETTY": "0", "FERRY_SHOW_WORK": "T",
			"FERRY_TIMEOUT": "1m30s", "FERRY_MODEL": "any model", "FERRY_WORLD": "cars,boats",
		},
		Want: nil,
	}, { // Test 2: A word that is not a bool.
		Env: map[string]string{"FERRY_JSON": "yes"}, Want: ErrValue, WantName: "FERRY_JSON",
	}, { // Test 3: A bad pretty value.
		Env: map[string]string{"FERRY_PRETTY": "maybe"}, Want: ErrValue, WantName: "FERRY_PRETTY",
	}, { // Test 4: A bad show-work value.
		Env:  map[string]string{"FERRY_SHOW_WORK": "on"},
		Want: ErrValue, WantName: "FERRY_SHOW_WORK",
	}, { // Test 5: A bool set to nothing.
		Env: map[string]string{"FERRY_JSON": ""}, Want: ErrValue, WantName: "FERRY_JSON",
	}, { // Test 6: A duration that does not parse.
		Env: map[string]string{"FERRY_TIMEOUT": "soon"}, Want: ErrValue, WantName: "FERRY_TIMEOUT",
	}, { // Test 7: Strings and lists take anything.
		Env: map[string]string{
			"FERRY_AUDIENCE": "true or false", "FERRY_EFFORT": "soon", "FERRY_WORLD": "1,,2",
			"FERRY_LOG_LEVEL": "loud", "FERRY_FILE": "",
		},
		Want: nil,
	}, { // Test 8: With two bad values, the first in name order is the one reported.
		Env:  map[string]string{"FERRY_TIMEOUT": "soon", "FERRY_JSON": "yes"},
		Want: ErrValue, WantName: "FERRY_JSON",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			Reset()
			clearEnv(t)
			for name, value := range test.Env {
				t.Setenv(name, value)
			}
			err := CheckEnv()
			if !errors.Is(err, test.Want) {
				t.Fatalf("CheckEnv() = %v, want %v", err, test.Want)
			}
			if err != nil && !strings.Contains(err.Error(), test.WantName+":") {
				t.Errorf("CheckEnv error %q does not name %s", err, test.WantName)
			}
		})
	}
}

// TestResetPanics checks that a default that does not parse as its flag's type is a developer
// error.
func TestResetPanics(t *testing.T) {
	tests := []struct {
		Flag        *pflag.Flag
		Default     string
		WantMessage string
	}{{ // Test 0: A duration default that is not a duration.
		Flag: &FlagTimeout, Default: "soon", WantMessage: "default for --timeout",
	}, { // Test 1: A bool default that is not a bool.
		Flag: &FlagJSON, Default: "yes", WantMessage: "default for --json",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			orig := test.Flag.DefValue
			t.Cleanup(func() {
				test.Flag.DefValue = orig
				Reset()
			})
			test.Flag.DefValue = test.Default
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("Reset did not panic")
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, test.WantMessage) {
					t.Errorf("panic %q does not mention %q", msg, test.WantMessage)
				}
			}()
			Reset()
		})
	}
}

// TestWorld checks the list getter: empty by default, comma lists from the environment, and
// repeated flags that add up and beat the environment.
func TestWorld(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	clearEnv(t)
	tests := []struct {
		Env        string
		Flags      []string
		WantResult []string
	}{{ // Test 0: Nothing set leaves the choice to the engine.
		WantResult: nil,
	}, { // Test 1: The environment holds a comma list with stray spaces and gaps.
		Env: " cars, ,kitchens ", WantResult: []string{"cars", "kitchens"},
	}, { // Test 2: Repeated flags add up and beat the environment.
		Env: "cars", Flags: []string{"sports", "boats, trains"},
		WantResult: []string{"sports", "boats", "trains"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			Reset()
			clearEnv(t)
			if test.Env != "" {
				t.Setenv("FERRY_WORLD", test.Env)
			}
			for _, raw := range test.Flags {
				setFlag(t, &FlagWorld, raw)
			}
			if diff := cmp.Diff(test.WantResult, World(), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReset checks that Reset restores every default and clears the set marks.
func TestReset(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	clearEnv(t)
	setFlag(t, &FlagModel, "other-model")
	setFlag(t, &FlagJSON, "true")
	setFlag(t, &FlagTimeout, "1s")
	setFlag(t, &FlagWorld, "cars,kitchens")
	Reset()
	for key, f := range flags {
		if f.Changed {
			t.Errorf("--%s still marked set after Reset", key)
		}
		if diff := cmp.Diff(f.DefValue, f.Value.String()); diff != "" {
			t.Errorf("--%s value mismatch after Reset (-want +got):\n%s", key, diff)
		}
	}
	if diff := cmp.Diff([]string(nil), World(), cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("World mismatch after Reset (-want +got):\n%s", diff)
	}
}
