package main

import (
	"testing"

	"github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo"
)

func TestRunVersionAndBuildInfo(t *testing.T) {
	info := buildinfo.Current()
	if info.Version == "" || info.GoVersion == "" || info.OS == "" || info.Arch == "" {
		t.Fatalf("info=%#v", info)
	}
	if err := runVersion(nil); err != nil {
		t.Fatal(err)
	}
	if err := runVersion([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	if err := runVersion([]string{"--unknown"}); err == nil {
		t.Fatal("expected unknown option error")
	}
}
