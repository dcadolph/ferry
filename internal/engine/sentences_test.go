package engine

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestSentenceCount checks the sentence counter on the cases the final step relies on.
func TestSentenceCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        string
		WantCount int
	}{{ // Test 0: Empty text.
		In: "", WantCount: 0,
	}, { // Test 1: Only spaces.
		In: "   \n\t", WantCount: 0,
	}, { // Test 2: Only punctuation.
		In: "... !?", WantCount: 0,
	}, { // Test 3: One sentence.
		In: "We keep topping off the oil.", WantCount: 1,
	}, { // Test 4: Words with no end mark count as one sentence.
		In: "We keep topping off the oil", WantCount: 1,
	}, { // Test 5: Two sentences.
		In: "We keep topping off the oil. The leak gets worse.", WantCount: 2,
	}, { // Test 6: Three sentences with every end mark.
		In: "Is the leak getting worse? It is! Fix it now.", WantCount: 3,
	}, { // Test 7: A trailing sentence with no end mark still counts.
		In: "The leak grows. Fix it now", WantCount: 2,
	}, { // Test 8: e.g. does not end a sentence.
		In: "Small fixes, e.g. another quart, buy time. Then the engine fails.", WantCount: 2,
	}, { // Test 9: i.e. does not end a sentence.
		In: "Fix the cause, i.e. the leak. Then stop buying oil.", WantCount: 2,
	}, { // Test 10: vs. does not end a sentence.
		In: "It is a quart vs. a gasket. The gasket wins.", WantCount: 2,
	}, { // Test 11: etc. does not end a sentence.
		In: "Oil, filters, etc. add up. The repair costs less.", WantCount: 2,
	}, { // Test 12: Abbreviations match without regard to case or a leading bracket.
		In: "Patches (E.G. quick fixes) pile up. We pay later.", WantCount: 2,
	}, { // Test 13: A decimal does not end a sentence.
		In: "Each patch costs 2.5 hours. That adds up.", WantCount: 2,
	}, { // Test 14: Three dots do not end a sentence.
		In: "We topped it off... and again. Then it seized.", WantCount: 2,
	}, { // Test 15: An ellipsis character does not end a sentence.
		In: "We topped it off… and again. Then it seized.", WantCount: 2,
	}, { // Test 16: Text that ends on an ellipsis is one sentence.
		In: "And then it seized...", WantCount: 1,
	}, { // Test 17: A closing quote after the end mark still ends the sentence.
		In: `The mechanic said "fix the leak." We did not.`, WantCount: 2,
	}, { // Test 18: A closing bracket after the end mark still ends the sentence.
		In: "We waited (too long.) The engine seized.", WantCount: 2,
	}, { // Test 19: A run of end marks ends one sentence.
		In: "Really?! Yes. Fix it.", WantCount: 3,
	}, { // Test 20: A period with no space after it does not end a sentence.
		In: "See example.com for details. Then call.", WantCount: 2,
	}, { // Test 21: Four sentences.
		In: "One. Two. Three. Four.", WantCount: 4,
	}, { // Test 22: A title does not end a sentence.
		In: "Dr. Smith fixed it. We paid.", WantCount: 2,
	}, { // Test 23: U.S. does not end a sentence.
		In: "The U.S. team shipped. We lost.", WantCount: 2,
	}, { // Test 24: p.m. does not end a sentence.
		In: "We ship at 5 p.m. every day. It breaks by 6.", WantCount: 2,
	}, { // Test 25: A quoted question in the middle of a sentence does not end it.
		In: `He asked, "Why?" and left. We stayed.`, WantCount: 2,
	}, { // Test 26: Mr. does not end a sentence.
		In: "Is it Mr. Lee's call? Yes.", WantCount: 2,
	}, { // Test 27: No. before a number does not end a sentence.
		In: "No. 1 priority is the leak. Fix it.", WantCount: 2,
	}, { // Test 28: A spoken line with a quoted question in its first sentence.
		In: `The mechanic asks "top it off again?" and we say yes. ` +
			"The leak keeps growing. So we book the repair this week.",
		WantCount: 3,
	}, { // Test 29: No on its own is a whole sentence.
		In: "Do we keep patching? No. We fix it.", WantCount: 3,
	}, { // Test 30: Mrs., Ms., St., and a.m. do not end a sentence.
		In: "Mrs. Lee and Ms. Ray met on Main St. at 9 a.m. sharp. They agreed.", WantCount: 2,
	}, { // Test 31: A sentence can start with a digit.
		In: "We lost 3 days. 4 more would sink the release.", WantCount: 2,
	}, { // Test 32: A sentence can start with an opening quote.
		In: `We asked for a week. "No," they said.`, WantCount: 2,
	}, { // Test 33: An end mark before a lowercase word does not end a sentence.
		In: "We shipped it. and then it broke.", WantCount: 1,
	}, { // Test 34: Trailing space after the last end mark changes nothing.
		In: "The leak grows. Fix it now.  \n", WantCount: 2,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantCount, sentenceCount(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
