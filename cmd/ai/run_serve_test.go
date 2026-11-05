package main

import "testing"

func TestValidateLoopbackAddr(t *testing.T) {
	valid := []string{
		"127.0.0.1:8765",
		"localhost:8765",
		"[::1]:8765",
	}
	for _, value := range valid {
		if err := validateLoopbackAddr(value); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}

	invalid := []string{
		"0.0.0.0:8765",
		"192.168.1.10:8765",
		":8765",
		"localhost:0",
		"localhost:99999",
		"localhost",
	}
	for _, value := range invalid {
		if err := validateLoopbackAddr(value); err == nil {
			t.Fatalf("expected %s to be rejected", value)
		}
	}
}

func TestParseRunServeArgsDefaultsAndOverrides(t *testing.T) {
	options, err := parseRunServeArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if options.addr != "127.0.0.1:8765" {
		t.Fatalf("default addr = %q", options.addr)
	}

	options, err = parseRunServeArgs([]string{
		"--addr", "localhost:9000",
		"--events-dir", "./events",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.addr != "localhost:9000" || options.eventsDir != "./events" {
		t.Fatalf("options = %#v", options)
	}
}


func TestParseRunServeArgsCollaborationStoreOverrides(t *testing.T) {
	options, err := parseRunServeArgs([]string{
		"--artifacts-dir", "./artifacts",
		"--messages-dir", "./messages",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.artifactsDir != "./artifacts" || options.messagesDir != "./messages" {
		t.Fatalf("options=%#v", options)
	}
}


func TestParseRunServeArgsPromotionStoreOverride(t *testing.T) {
	options, err := parseRunServeArgs([]string{"--promotion-dir", "./promotion"})
	if err != nil {
		t.Fatal(err)
	}
	if options.promotionDir != "./promotion" {
		t.Fatalf("options=%#v", options)
	}
}


func TestParseRunServeArgsControlStateOverrides(t *testing.T) {
	options, err := parseRunServeArgs([]string{
		"--state-dir", "./runs",
		"--checkpoint-dir", "./checkpoints",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.stateDir != "./runs" || options.checkpointDir != "./checkpoints" {
		t.Fatalf("options=%#v", options)
	}
}
