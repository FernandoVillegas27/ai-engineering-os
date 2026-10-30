package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

var (
	parenDuration = regexp.MustCompile(`\(\d+(?:\.\d+)?s\)`)
	tabDuration   = regexp.MustCompile(`(?m)(\t)\d+(?:\.\d+)?s$`)
	tempDirSuffix = regexp.MustCompile(`([\\/]Test[A-Za-z0-9_]*?)\d{6,}`)
	hexAddress    = regexp.MustCompile(`0x[0-9a-fA-F]{5,}`)
	hexOffset     = regexp.MustCompile(`\+0x[0-9a-fA-F]+`)
	goroutineID   = regexp.MustCompile(`goroutine \d+`)
)

// NormalizeOutput removes only values that vary between identical runs.
// Line numbers, file names and messages are deliberately kept, so any real
// change in a failure alters its signature instead of being hidden.
// Roots are workspace paths that differ between the baseline and the staged copy.
func NormalizeOutput(output string, roots ...string) string {
	text := strings.ReplaceAll(output, "\r\n", "\n")

	var variants []string
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		variants = append(variants, root, strings.ReplaceAll(root, `\`, "/"), strings.ReplaceAll(root, "/", `\`))
	}
	sort.Slice(variants, func(i, j int) bool { return len(variants[i]) > len(variants[j]) })
	for _, variant := range variants {
		text = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(variant)).ReplaceAllString(text, "<workspace>")
	}

	text = parenDuration.ReplaceAllString(text, "(<duration>)")
	text = tabDuration.ReplaceAllString(text, "${1}<duration>")
	text = tempDirSuffix.ReplaceAllString(text, "${1}<n>")
	text = hexOffset.ReplaceAllString(text, "+0x<offset>")
	text = hexAddress.ReplaceAllString(text, "0x<addr>")
	text = goroutineID.ReplaceAllString(text, "goroutine <n>")

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Signature returns the hex SHA-256 of the normalized output.
func Signature(output string, roots ...string) string {
	sum := sha256.Sum256([]byte(NormalizeOutput(output, roots...)))
	return hex.EncodeToString(sum[:])
}
