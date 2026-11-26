//go:build windows

package main

import "github.com/lxn/walk"

// The log dock: a fixed four-line console pinned to the very bottom of the
// window on EVERY page. The page grid ends above it (pageGrid reserves
// logDockH before the 700-tall fit factor runs), so no page can paint over it
// and it never covers page content. It always follows the newest lines — the
// full history, scrolling and copying live one click away on the Logs page,
// which is exactly where a click on the dock lands.
//
// It is chrome, not page content: paintShell draws it after the page body with
// the transition offset cleared, so it stays put while a page slides in, and
// dropdown popups still paint above it.
const (
	logDockLines = 4
	logDockLineH = 22.0
	logDockPad   = 8.0
	logDockH     = logDockLines*logDockLineH + 2*logDockPad
)

// dockLogs returns the newest lines the dock can show, oldest first so the
// newest sits on the bottom row — the same reading order as the run-log page.
func (u *natUI) dockLogs() []natLog {
	all := u.m.logs
	if len(all) > logDockLines {
		all = all[len(all)-logDockLines:]
	}
	return all
}

// paintLogDock draws the bottom strip. The background is a full-bleed device
// pixel rect (like the sidebar pane) so its edges stay exact at any window
// size or DPI; the lines themselves are design-grid content. The whole strip
// is one spot that opens the Logs page.
func (u *natUI) paintLogDock(c *walk.Canvas) {
	b := u.mw.ClientBoundsPixels()
	top := u.oy + u.s(u.dh+capH)
	if top < 0 {
		top = 0
	}
	h := b.Height - top
	if h <= 0 || b.Width <= 0 {
		return
	}
	bg := th.inset
	if u.m.hover == "go:logs" {
		bg = mix(th.inset, th.cardHover, 0.55)
	}
	u.fillRect(c, bg, pxRect(0, top, b.Width, h))
	u.fillRect(c, th.line2, pxRect(0, top, b.Width, 1))

	// Right cell: the open-logs affordance, an icon over the page's own label.
	// It brightens with the strip on hover so the dock reads as clickable.
	const cellW = 64.0
	cx := u.dw - cellW
	u.fillRect(c, th.line2, u.r(cx, u.dh+12, 1, logDockH-24))
	col := th.faint
	if u.m.hover == "go:logs" {
		col = th.accent
	}
	u.icon(c, "logs", cx+(cellW-18)/2, u.dh+33, 18, col)
	u.textC(c, tr("日志"), u.f.f8, col, u.r(cx, u.dh+55, cellW, 16))

	lines := u.dockLogs()
	msgW := cx - 142 - 12
	if msgW < 40 {
		msgW = 40
	}
	if len(lines) == 0 {
		u.txt(c, 16, u.dh+logDockPad, cx-28, logDockLineH, tr("暂无日志"), u.f.fmono, th.faint)
	}
	y := u.dh + logDockPad
	for _, l := range lines {
		tag, pillBg, pillCol, msgCol := logLevelStyle(l)
		if tag == "INFO" {
			// The dock surface IS the inset colour, so the run-log's inset
			// pill would vanish into it; the raised card colour keeps it.
			pillBg = th.card
		}
		u.txt(c, 16, y, 62, logDockLineH, l.at.Format("15:04:05"), u.f.fmono, th.faint)
		pill := u.r(84, y+4, 50, logDockLineH-8)
		u.fillRound(c, pillBg, pill, 4)
		u.textC(c, tag, u.f.f8, pillCol, pill)
		u.txt(c, 142, y, msgW, logDockLineH, l.text, u.f.fmono, msgCol)
		y += logDockLineH
	}
	u.addSpot("go:logs", pxRect(0, top, b.Width, h), true)
}
