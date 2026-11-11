package cmd

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestResolveVersion checks that a build stamp wins and that an unstamped test binary, which
// has no module version, reports dev. It runs serially because it changes the package-level
// version.
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		Stamp      string
		WantResult string
	}{{ // Test 0: A stamped version is returned as is.
		Stamp: "v1.2.3", WantResult: "v1.2.3",
	}, { // Test 1: The default stamp falls back to dev.
		Stamp: "dev", WantResult: "dev",
	}, { // Test 2: An empty stamp falls back to dev.
		Stamp: "", WantResult: "dev",
	}}
	orig := version
	t.Cleanup(func() { version = orig })
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			version = test.Stamp
			if diff := cmp.Diff(test.WantResult, resolveVersion()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
