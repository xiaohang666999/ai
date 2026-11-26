//go:build windows

package main

import (
	"fmt"
	"math"
	"testing"
)

// TestPageGridReservesDock pins the fit engine's contract at every supported
// window size and DPI: the page area is always at least the 700-tall design
// grid, and page area + caption band + log dock exactly fill the client
// height at the frame's scale. That is what keeps every page's bottom edge
// (and the sidebar foot) above the dock instead of painting under it.
func TestPageGridReservesDock(t *testing.T) {
	sizes := [][2]float64{
		{1280, 860}, // preferred startup
		{880, 664},  // the supported floor
		{1024, 600}, // below the floor: the grid must shrink, not clip
		{1920, 1080},
		{2560, 1440},
		{880, 560}, // the old floor, now short by exactly the dock
	}
	for _, dpi := range []float64{96, 120, 144, 192} {
		for _, s := range sizes {
			pxW := int(s[0] * dpi / 96)
			pxH := int(s[1] * dpi / 96)
			k, dw, dh := pageGrid(pxW, pxH, dpi)
			if k <= 0 || math.IsNaN(k) {
				t.Fatalf("dpi %.0f %vx%v: bad scale k=%v", dpi, s[0], s[1], k)
			}
			if dh < 700 {
				t.Fatalf("dpi %.0f %vx%v: page area %v under the 700 grid", dpi, s[0], s[1], dh)
			}
			if got, want := dh+capH+logDockH, float64(pxH)/k; math.Abs(got-want) > 1e-6 {
				t.Fatalf("dpi %.0f %vx%v: page+caption+dock = %v, client = %v", dpi, s[0], s[1], got, want)
			}
			if got, want := dw, float64(pxW)/k; math.Abs(got-want) > 1e-6 {
				t.Fatalf("dpi %.0f %vx%v: dw = %v, want %v", dpi, s[0], s[1], got, want)
			}
		}
	}
}

// TestPageGridTinyClient guards the transient 0x0-ish client rectangles a
// resize or DPI change can hand paint: the scale must stay positive so one
// odd frame cannot poison hit-testing or font sizing.
func TestPageGridTinyClient(t *testing.T) {
	for _, px := range [][2]int{{10, 10}, {10, 40}, {200, 34}, {0, 0}} {
		k, _, _ := pageGrid(px[0], px[1], 96)
		if k <= 0 || math.IsNaN(k) || math.IsInf(k, 0) {
			t.Fatalf("client %v: k = %v, want a positive finite scale", px, k)
		}
	}
}

// TestDockHeightFitsFourLines pins the dock's own budget: exactly four rows
// plus padding, no more — the strip is a glanceable tail, not a second log
// page, and the window floor is derived from this height.
func TestDockHeightFitsFourLines(t *testing.T) {
	if logDockLines != 4 {
		t.Fatalf("dock shows %d lines, want 4", logDockLines)
	}
	if got := logDockPad + logDockLines*logDockLineH + logDockPad; got != logDockH {
		t.Fatalf("four rows end at %v, dock height is %v", got, logDockH)
	}
}

// TestDockShowsNewestFour pins the tail selection: oldest first, newest on
// the bottom row, and a short history is shown whole rather than padded.
func TestDockShowsNewestFour(t *testing.T) {
	u := &natUI{m: newNatModel()}
	if got := len(u.dockLogs()); got != 0 {
		t.Fatalf("empty history shows %d lines, want 0", got)
	}
	for i := 0; i < 3; i++ {
		u.appendLog(fmt.Sprintf("line %d", i), 0, false, false)
	}
	if got := len(u.dockLogs()); got != 3 {
		t.Fatalf("short history shows %d lines, want all 3", got)
	}
	for i := 3; i < 10; i++ {
		u.appendLog(fmt.Sprintf("line %d", i), 0, false, false)
	}
	lines := u.dockLogs()
	if len(lines) != 4 {
		t.Fatalf("dock shows %d lines, want 4", len(lines))
	}
	for i, l := range lines {
		if want := fmt.Sprintf("line %d", 6+i); l.text != want {
			t.Fatalf("row %d = %q, want %q (newest last)", i, l.text, want)
		}
	}
}
