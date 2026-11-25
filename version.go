package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// This client's own home. It is deliberately left unset: the repository, the
// release feed, the issue tracker and the remote stop list all belong to a
// deployment, not to the program, so nothing here names a personal account.
// Every consumer treats an empty value as "not configured" and degrades
// quietly instead of pointing the About page at a stranger's repository.
const (
	repoOwner = ""
	repoName  = "warp-client"
	repoURL   = ""

	repoReleasesURL = ""
	repoIssuesURL   = ""
	repoNoticesURL  = ""
	repoLicenseURL  = ""
	// coreRepoURL is where the mihomo core comes from: the client downloads
	// its releases rather than building it, so this is the "core source" the
	// About card names instead of a path on this disk.
	coreRepoURL = "https://github.com/MetaCubeX/mihomo"
)

const (
	ghLatestURL = ""

	updateCheckTimeout = 2 * time.Second
	updateCacheTTL     = 6 * time.Hour
	updateCacheName    = "warpscout-latest-version"
)

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

func noticeUpdate(ctx context.Context) {
	cur := resolveVersion()
	latest := latestVersion(ctx)
	if !newerVersion(latest, cur) {
		return
	}
	writeUpdateBanner(os.Stderr, newConStyles(consoleRenderer(os.Stderr)), cur, latest)
}

func writeUpdateBanner(w io.Writer, st conStyles, cur, latest string) {
	line := st.warn.Bold(true).Render("Update available") +
		st.dim.Render(": "+cur+" → ") + st.accent.Bold(true).Render(latest)
	fmt.Fprintln(w, st.box.Render(line))
	fmt.Fprintln(w)
}

// An empty answer means "unknown" - the caller stays quiet rather than failing a
// scan over a version check.
func latestVersion(ctx context.Context) string {
	path := updateCachePath()
	if v, ok := readVersionCache(path, updateCacheTTL); ok {
		return v
	}
	v, _ := fetchLatestVersion(ctx)
	// Cached even when the fetch failed: on a network where GitHub is blocked
	// every run would otherwise pay the timeout.
	writeVersionCache(path, v)
	return v
}

// In the temp dir rather than os.UserCacheDir(): on OpenWrt that is /root/.cache
// on the flash overlay, and this file is disposable. The uid keeps a shared /tmp
// from letting the first user own it and every other one silently fail to write.
func updateCachePath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d", updateCacheName, os.Getuid()))
}

func readVersionCache(path string, ttl time.Duration) (string, bool) {
	if path == "" || ttl <= 0 {
		return "", false
	}
	fi, err := os.Stat(path)
	if err != nil || time.Since(fi.ModTime()) > ttl {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func writeVersionCache(path, version string) {
	if path == "" {
		return
	}
	os.WriteFile(path, []byte(version+"\n"), 0644)
}

// fetchLatestVersion returns the newest release tag and whether the answer came
// from GitHub at all. The two are separate because "no release has been
// published yet" (a 404 on a repository that exists) and "GitHub is
// unreachable" are different facts, and the update card must not report the
// first one as the second.
func fetchLatestVersion(ctx context.Context) (version string, answered bool) {
	// No release feed is configured: stay silent rather than guess at one.
	if ghLatestURL == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghLatestURL, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// The repository answered: it simply has no releases yet.
		return "", true
	}
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var r struct {
		Tag string `json:"tag_name"`
	}
	if json.NewDecoder(resp.Body).Decode(&r) != nil {
		return "", true
	}
	return strings.TrimPrefix(r.Tag, "v"), true
}

func newerVersion(latest, current string) bool {
	l, ok := parseSemver(latest)
	if !ok {
		return false
	}
	c, ok := parseSemver(current)
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseSemver accepts "1", "1.0" and "1.0.0" alike: this client's releases are
// numbered "XEHN 1.0" style, and requiring three parts would silently disable
// the update notice for every build named that way.
func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) == 0 || len(parts) > len(out) {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
