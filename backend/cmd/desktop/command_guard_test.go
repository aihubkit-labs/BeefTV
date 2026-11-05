package main

import (
	"strings"
	"testing"

	"infinite-canvas/backend/internal/buildinfo"
)

func TestDesktopBrandUsesItsSkillSchemeAndDisplayName(t *testing.T) {
	original := buildinfo.ProductScheme
	t.Cleanup(func() { buildinfo.ProductScheme = original; buildinfo.SetDisplayName("") })
	buildinfo.ProductScheme = "zmodeltv"
	buildinfo.SetDisplayName("ZModelTV")
	if err := validateDesktopArgs([]string{"zmodeltv://skills/install?id=storyboard&version=r_123"}); err != nil {
		t.Fatal(err)
	}
	if err := validateDesktopArgs([]string{"mcp"}); err == nil || !strings.Contains(err.Error(), "ZModelTV") {
		t.Fatal(err)
	}
}

func TestDesktopRejectsCLICommands(t *testing.T) {
	for _, args := range [][]string{{"mcp", "serve"}, {"ops", "--json"}, {"canvas", "get"}, {"client", "register"}, {"unknown-command"}, {"--json", "mcp", "serve"}} {
		if err := validateDesktopArgs(args); err == nil {
			t.Fatalf("desktop accepted CLI arguments %v", args)
		}
	}
	for _, args := range [][]string{nil, {"-psn_0_12345"}, {"--disable-gpu"}, {"--user-data-dir=platform-profile"}, {"-NSDocumentRevisionsDebugMode", "YES"}, {"-AppleLanguages", "(en)"}} {
		if err := validateDesktopArgs(args); err != nil {
			t.Fatalf("desktop rejected platform arguments %v: %v", args, err)
		}
	}
}
