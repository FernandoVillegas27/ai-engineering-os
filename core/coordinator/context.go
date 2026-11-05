package coordinator

import "strings"

const DefaultCombinedContextMax = 24 * 1024

// MergeContext combines durable and run-local evidence without allowing either
// source to silently erase the other. Both inputs are already treated as
// untrusted evidence by runtime prompts.
func MergeContext(existing, dynamic string, maxBytes int) string {
	existing = strings.TrimSpace(existing)
	dynamic = strings.TrimSpace(dynamic)
	if existing == "" {
		return boundedContext(dynamic, maxBytes)
	}
	if dynamic == "" {
		return boundedContext(existing, maxBytes)
	}
	return boundedContext(existing+"\n\n"+dynamic, maxBytes)
}

func boundedContext(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 {
		maxBytes = DefaultCombinedContextMax
	}
	if maxBytes > 64*1024 {
		maxBytes = 64 * 1024
	}
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 3 {
		return value[:maxBytes]
	}
	cut := maxBytes - 3
	for cut > 0 && (value[cut]&0xC0) == 0x80 {
		cut--
	}
	return value[:cut] + "..."
}
