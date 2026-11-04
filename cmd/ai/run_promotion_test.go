package main

import "testing"

func TestParseRunPromotionArgs(t *testing.T) {
	options, err := parseRunPromotionArgs([]string{
		"run-123",
		"--promotion-dir", "./candidates",
		"--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.runID != "run-123" || options.promotionDir != "./candidates" || !options.json {
		t.Fatalf("options = %#v", options)
	}
}

func TestParseRunPromotionArgsRejectsInvalidInput(t *testing.T) {
	cases := [][]string{
		nil,
		{"run-1", "run-2"},
		{"--promotion-dir"},
		{"--unknown"},
	}
	for _, args := range cases {
		if _, err := parseRunPromotionArgs(args); err == nil {
			t.Fatalf("expected error for %#v", args)
		}
	}
}
