package cursor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	launchDirect          = "direct"
	launchWindowsNode     = "windows-node-bypass"
	launchBatchUnresolved = "windows-batch-unresolved"
)

// launchPlan describes how the Cursor agent process is started.
type launchPlan struct {
	Kind    string
	Command string
	Args    []string
	Env     []string
	Node    string
}

var versionDirPattern = regexp.MustCompile(`^(\d{4})\.(\d{1,2})\.(\d{1,2})(-\d{2}-\d{2}-\d{2})?-[a-f0-9]+$`)

// planAgentLaunch avoids cmd.exe, which truncates arguments at the first newline.
func planAgentLaunch(goos, resolved string) launchPlan {
	plan := launchPlan{Kind: launchDirect, Command: resolved}
	if !strings.EqualFold(goos, "windows") {
		return plan
	}
	switch strings.ToLower(filepath.Ext(resolved)) {
	case ".cmd", ".bat", ".ps1":
	default:
		return plan
	}

	node, entry, ok := locateCursorRuntime(filepath.Dir(resolved))
	if !ok {
		plan.Kind = launchBatchUnresolved
		return plan
	}
	return launchPlan{
		Kind:    launchWindowsNode,
		Command: node,
		Args:    []string{entry},
		Env:     []string{"CURSOR_INVOKED_AS=" + filepath.Base(resolved)},
		Node:    node,
	}
}

// locateCursorRuntime mirrors the selection logic of the official cursor-agent.ps1 wrapper.
func locateCursorRuntime(installDir string) (node string, entry string, ok bool) {
	if found, entryPath, exists := runtimeFiles(installDir); exists {
		return found, entryPath, true
	}

	entries, err := os.ReadDir(filepath.Join(installDir, "versions"))
	if err != nil {
		return "", "", false
	}
	type candidate struct {
		name string
		date int
	}
	var candidates []candidate
	for _, item := range entries {
		match := versionDirPattern.FindStringSubmatch(item.Name())
		if !item.IsDir() || match == nil {
			continue
		}
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		candidates = append(candidates, candidate{name: item.Name(), date: year*10000 + month*100 + day})
	}
	if len(candidates) == 0 {
		return "", "", false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].date != candidates[j].date {
			return candidates[i].date > candidates[j].date
		}
		return candidates[i].name > candidates[j].name
	})
	return runtimeFiles(filepath.Join(installDir, "versions", candidates[0].name))
}

func runtimeFiles(dir string) (node string, entry string, ok bool) {
	node = filepath.Join(dir, "node.exe")
	entry = filepath.Join(dir, "index.js")
	if isRegularFile(node) && isRegularFile(entry) {
		return node, entry, true
	}
	return "", "", false
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// promptNeedsSafeTransport reports characters that a cmd.exe batch launcher would corrupt.
func promptNeedsSafeTransport(prompt string) bool {
	return strings.ContainsAny(prompt, "\r\n\"%&|<>^!")
}
