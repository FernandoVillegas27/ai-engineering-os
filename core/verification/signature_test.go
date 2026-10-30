package verification

import (
	"strings"
	"testing"
)

func TestSignatureIgnoresOnlyVolatileValues(t *testing.T) {
	base := "=== RUN   TestX\n    x_test.go:12: boom\n--- FAIL: TestX (0.01s)\nFAIL\tpkg\t0.152s\n"
	for name, variant := range map[string]string{
		"parenthesized duration": strings.ReplaceAll(base, "(0.01s)", "(3.40s)"),
		"package duration":       strings.ReplaceAll(base, "0.152s", "12.9s"),
		"crlf":                   strings.ReplaceAll(base, "\n", "\r\n"),
		"trailing spaces":        strings.ReplaceAll(base, "boom\n", "boom   \n"),
	} {
		if Signature(variant) != Signature(base) {
			t.Fatalf("%s must not change the signature", name)
		}
	}
}

func TestSignatureKeepsEverythingThatCouldRevealARegression(t *testing.T) {
	base := "    x_test.go:12: boom\n"
	for name, variant := range map[string]string{
		"line number": "    x_test.go:13: boom\n",
		"file name":   "    y_test.go:12: boom\n",
		"message":     "    x_test.go:12: bang\n",
		"extra line":  base + "    x_test.go:14: also broken\n",
	} {
		if Signature(variant) == Signature(base) {
			t.Fatalf("%s must change the signature", name)
		}
	}
}

func TestNormalizeOutputReplacesWorkspaceRootsInAllPathStyles(t *testing.T) {
	root := `C:\Git Fernando\.ai-engineering-os-stage-111`
	text := `at C:\Git Fernando\.ai-engineering-os-stage-111\a_test.go:3 and C:/Git Fernando/.ai-engineering-os-stage-111/b_test.go:4`
	got := NormalizeOutput(text, root)
	if strings.Contains(got, "stage-111") || strings.Count(got, "<workspace>") != 2 {
		t.Fatalf("got=%q", got)
	}
	other := strings.ReplaceAll(text, "stage-111", "baseline-222")
	if Signature(other, strings.ReplaceAll(root, "stage-111", "baseline-222")) != Signature(text, root) {
		t.Fatal("different workspace roots must yield the same signature")
	}
}

func TestNormalizeOutputHandlesRandomTempDirsAddressesAndGoroutines(t *testing.T) {
	a := `open C:\Temp\TestCopyFile2618743208\001\x: denied` + "\ngoroutine 6 [running]:\nfn(0x7ff7058b7400)\n\tf.go:10 +0x239\n"
	b := `open C:\Temp\TestCopyFile9999999999\001\x: denied` + "\ngoroutine 17 [running]:\nfn(0x7ff7000000aa)\n\tf.go:10 +0x1c0\n"
	if Signature(a) != Signature(b) {
		t.Fatalf("volatile values leaked into the signature:\n%s\n%s", NormalizeOutput(a), NormalizeOutput(b))
	}
}

func TestSignatureIsStableHexSHA256(t *testing.T) {
	sig := Signature("anything")
	if len(sig) != 64 || strings.Trim(sig, "0123456789abcdef") != "" {
		t.Fatalf("sig=%q", sig)
	}
	if sig != Signature("anything") {
		t.Fatal("signature must be deterministic")
	}
}
