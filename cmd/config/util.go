package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// envPrefix namespaces the environment variables the config package reads.
const envPrefix = "FERRY_"

// envKey maps a flag key like log-level to FERRY_LOG_LEVEL.
func envKey(key string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
}

// load resolves key by priority: command-line flag, then environment, then def.
func load(key, def string) string {
	if f, ok := flags[key]; ok && f.Changed {
		return f.Value.String()
	}
	if v, ok := os.LookupEnv(envKey(key)); ok {
		return v
	}
	return def
}

// loadString resolves key as a string.
func loadString(key, def string) string {
	return load(key, def)
}

// loadBool resolves key as a bool. It falls back to def when the raw value does not parse,
// which CheckEnv rejects before any getter runs.
func loadBool(key string, def bool) bool {
	b, err := strconv.ParseBool(load(key, strconv.FormatBool(def)))
	if err != nil {
		return def
	}
	return b
}

// loadDuration resolves key as a duration. It falls back to def when the raw value does not
// parse, which CheckEnv rejects before any getter runs.
func loadDuration(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(load(key, def.String()))
	if err != nil {
		return def
	}
	return d
}

// loadStrings resolves key as a comma-separated list.
func loadStrings(key string, def []string) []string {
	return splitList(load(key, strings.Join(def, ",")))
}

// splitList splits a comma-separated value into trimmed, non-empty items.
func splitList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
