package engine

import (
	"strings"
	"unicode"
)

// Runes that can sit between an end mark and the space after it, and before the first word of
// the next sentence.
const (
	// closers are the quotes and brackets that can close right after an end mark.
	closers = `"')]}”’»`
	// openers are the quotes and brackets that can open a sentence.
	openers = `"'([{“‘«`
)

// sentenceCount returns how many sentences text holds. A sentence ends at a period, question
// mark, or exclamation point, once any closing quotes or brackets are passed, when the text
// ends there or the next word starts with a capital letter or a digit. A mark followed by a
// lowercase word, such as the one inside a quoted question in the middle of a sentence, does
// not end it. Ellipses, decimals, and common abbreviations such as e.g., etc., Dr., and p.m.
// do not end a sentence either. Words after the last end mark count as one more.
func sentenceCount(text string) int {
	rs := []rune(text)
	count, words := 0, false
	for i := 0; i < len(rs); i++ {
		if !isEndMark(rs[i]) {
			if unicode.IsLetter(rs[i]) || unicode.IsDigit(rs[i]) {
				words = true
			}
			continue
		}
		end := i
		for end < len(rs) && isEndMark(rs[end]) {
			end++
		}
		next := end
		for next < len(rs) && strings.ContainsRune(closers, rs[next]) {
			next++
		}
		first, more := nextWordStart(rs[next:])
		atBreak := !more || (unicode.IsSpace(rs[next]) && startsSentence(first))
		marks := string(rs[i:end])
		abbreviated := marks == "." && isAbbreviation(rs[:i], first)
		if atBreak && words && !isEllipsis(marks) && !abbreviated {
			count++
			words = false
		}
		i = next - 1
	}
	if words {
		count++
	}
	return count
}

// nextWordStart returns the first rune of rest that is not a space or an opening quote or
// bracket, and false when rest holds nothing else.
func nextWordStart(rest []rune) (rune, bool) {
	for _, r := range rest {
		if !unicode.IsSpace(r) && !strings.ContainsRune(openers, r) {
			return r, true
		}
	}
	return 0, false
}

// startsSentence reports whether r can be the first rune of a sentence: a digit, or a letter
// that is not lowercase.
func startsSentence(r rune) bool {
	return unicode.IsDigit(r) || (unicode.IsLetter(r) && !unicode.IsLower(r))
}

// isEndMark reports whether r can end a sentence.
func isEndMark(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == '…'
}

// isEllipsis reports whether the run of end marks is an ellipsis.
func isEllipsis(marks string) bool {
	return strings.Count(marks, ".") > 1 || strings.ContainsRune(marks, '…')
}

// isAbbreviation reports whether before ends in a word that a period does not end a sentence
// after, such as e.g., etc., Dr., or p.m. The word no counts only when next, the first rune of
// the word after it, is a digit, as in No. 1, since on its own it is a whole sentence.
func isAbbreviation(before []rune, next rune) bool {
	start := len(before)
	for start > 0 && !unicode.IsSpace(before[start-1]) {
		start--
	}
	word := strings.TrimLeft(string(before[start:]), openers)
	switch strings.ToLower(word) {
	case "e.g", "i.e", "vs", "etc", "mr", "mrs", "ms", "dr", "st", "a.m", "p.m", "u.s":
		return true
	case "no":
		return unicode.IsDigit(next)
	default:
		return false
	}
}
