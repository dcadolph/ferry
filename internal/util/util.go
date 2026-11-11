// Package util holds the small helpers that more than one ferry package needs.
package util

import "strings"

// CollapseSpace trims s and collapses each run of whitespace inside it, line breaks included,
// to a single space.
func CollapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Inc returns i plus one. Templates use it to number a list from one.
func Inc(i int) int {
	return i + 1
}
