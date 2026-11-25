package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
)

// coreRepoURL is where the mihomo core comes from: the client downloads its
// releases rather than building it, so this is the "core source" the About card
// names instead of a path on this disk. It is the only address baked into the
// program — the project has no self-update channel and no remote control, so
// nothing here points at a repository of its own.
const coreRepoURL = "https://github.com/MetaCubeX/mihomo"

// Set through -ldflags "-X main.version=" from the git tag; see VERSIONING.md.
var version = "dev"

// windowsGUIBuild is "1" only in the -H windowsgui build (pack.ps1 injects
// it). The UAC proxy helper may only run from that build: relaunching a
// console binary elevated would flash a terminal window on screen.
var windowsGUIBuild = "0"

// buildStamp is the packaging time ("20261029-2252"), injected by pack.ps1.
// Several builds can share one version number while a release is being
// prepared, and "which build am I running" then becomes a real question — the
// About page shows this next to the version so it can be answered at a glance.
var buildStamp = ""

func runVersionCmd(context.Context, options) error {
	fmt.Println(resolveVersion())
	return nil
}

// versionWithBuild is what the About page shows: the version, plus the build
// stamp when the binary was packaged by pack.ps1.
func versionWithBuild() string {
	v := shortVersion()
	if buildStamp == "" {
		return v
	}
	return v + " · " + buildStamp
}

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	// "go install ...@latest" embeds the module version; a plain "go build" gives "(devel)".
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return version
}
