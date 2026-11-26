//go:build windows

package main

import (
	"github.com/lxn/walk"
)

// The log dock is a strip of the newest log lines pinned to the foot of the
// content column. It is drawn on every page, so the latest activity stays in
// view wherever the user is. Pages stop at bodyBottom(), above the dock, and
// the dock never moves with the page-switch transition. A click opens the full
// run log.
const (
	logBoxH     = 88.0 // card height: the rows plus their padding
	logBoxGap   = 10.0 // space between the last page card and the dock
	logDockRows = 4
	logDockRowH = 19.0
)

// logDockBox is the dock's rectangle in design units. It sits in the content
// column, so the sidebar's own foot (core version) is never covered.
func (u *natUI) logDockBox() (x, y, w, h float64) {
	x, w = u.contentX(), u.contentW()
	h = logBoxH
	y = u.dh - shellBottomPad - logBoxH
	return
}

// paintLogDock draws the newest logDockRows lines, oldest on top, newest at the
// bottom, each with its time, level tag and message.
func (u *natUI) paintLogDock(c *walk.Canvas) {
	x, y, w, h := u.logDockBox()
	u.cardPlain(c, x, y, w, h)

	all := u.m.logs
	if len(all) == 0 {
		u.textC(c, tr("暂无日志"), u.f.f9, th.faint, u.r(x+16, y, w-32, h))
	} else {
		start := len(all) - logDockRows
		if start < 0 {
			start = 0
		}
		ly := y + (h-logDockRows*logDockRowH)/2
		for _, l := range all[start:] {
			tag, pillBg, pillCol, msgCol := logLevelStyle(l)
			u.txt(c, x+16, ly, 62, logDockRowH, l.at.Format("15:04:05"), u.f.fmono, th.faint)
			pill := u.r(x+82, ly+2, 46, logDockRowH-4)
			u.fillRound(c, pillBg, pill, 5)
			u.textC(c, tag, u.f.f8, pillCol, pill)
			u.txt(c, x+138, ly, w-154, logDockRowH, l.text, u.f.fmono, msgCol)
			ly += logDockRowH
		}
	}
	u.addSpot("log:dock", u.r(x, y, w, h), true)
}
