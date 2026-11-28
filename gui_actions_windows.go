//go:build windows

package main

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/xhwarp/warp-client/internal/core"
)

// actionPages routes every id that is not part of the connect pipeline (which
// action() handles). The prefixes are the contract between a paint function and
// this file: nav:, theme:, lang:, ruletab:, settab:, logtab:, mode:, node
// filter/sort, star:, set:, about:, go:.
func (u *natUI) actionPages(id string) {
	m := u.m
	switch {
	case strings.HasPrefix(id, "nav:"):
		u.gotoPage(strings.TrimPrefix(id, "nav:"))

	case strings.HasPrefix(id, "go:"):
		u.gotoPage(strings.TrimPrefix(id, "go:"))

	case id == "log:dock":
		// The dock is a summary of the run log: a click opens the full one.
		u.gotoPage(pageLogs)

	case id == "theme:cycle":
		u.cycleTheme()
	case strings.HasPrefix(id, "theme:"):
		v := strings.TrimPrefix(id, "theme:")
		u.mutateSettings(func(s *clientSettings) { s.Theme = v })
		u.applyTheme()

	case id == "lang:cycle":
		next := langEN
		if m.set.Lang == langEN {
			next = langZH
		}
		u.mutateSettings(func(s *clientSettings) { s.Lang = next })
		u.applyLang()

	case strings.HasPrefix(id, "lang:"):
		v := strings.TrimPrefix(id, "lang:")
		u.mutateSettings(func(s *clientSettings) { s.Lang = v })
		u.applyLang()

	case strings.HasPrefix(id, "pickpreset:"):
		v := strings.TrimPrefix(id, "pickpreset:")
		m.dropOpen = ""
		switch v {
		case modeBypass, modeBlacklist, modeGlobal, modeDirect, modeCustom:
		default:
			u.invalidate()
			return
		}
		// Bypass mainland is a ruleset over the geo databases; without them the
		// core refuses the whole config. Rather than leaving the row inert, the
		// click fetches the databases and applies the preset when they land.
		if v == modeBypass && !geoDataReady() {
			u.startGeoDownload()
			return
		}
		u.mutateSettings(func(s *clientSettings) { s.ProxyMode = v })

	case strings.HasPrefix(id, "logtab:"):
		m.logTab = strings.TrimPrefix(id, "logtab:")
		u.invalidate()

	case strings.HasPrefix(id, "nodefilter:"):
		m.nodeFilter = strings.TrimPrefix(id, "nodefilter:")
		m.scroll = 0
		u.invalidate()
	case strings.HasPrefix(id, "nodesort:"):
		m.nodeSort = strings.TrimPrefix(id, "nodesort:")
		m.scroll = 0
		u.invalidate()
	case id == "nodesearch":
		if v, ok := u.promptText(tr("搜索节点"), tr("名称、国家、城市或 IP"), m.nodeSearch); ok {
			m.nodeSearch = strings.TrimSpace(v)
			m.scroll = 0
			u.invalidate()
		}
	case id == "nodesearch:clear":
		m.nodeSearch = ""
		m.scroll = 0
		u.invalidate()

	case strings.HasPrefix(id, "star:"):
		ep := strings.TrimPrefix(id, "star:")
		u.mutateSettings(func(s *clientSettings) { s.Starred = toggleStar(s.Starred, ep) })

	case strings.HasPrefix(id, "set:"), strings.HasPrefix(id, "pickcapture:"):
		u.actionSetting(id)
	case strings.HasPrefix(id, "about:"):
		u.actionAbout(id)
	case id == "core:download":
		u.doCore()
	case id == "core:refresh":
		resetCoreCapabilityCache()
		u.addLog(tr("重新探测内核…"), false)
		go u.probeCore()
	}
}

// startGeoDownload fetches the geo databases the bypass preset needs and then
// applies it. Called on the GUI thread; the transfer runs off-thread and
// reports through the log panel so a slow download is visible rather than
// looking like a dead click.
func (u *natUI) startGeoDownload() {
	if u.m.geoBusy {
		return
	}
	dir, err := coreDir()
	if err != nil {
		u.addLog(tr("无法定位核心目录: ")+err.Error(), true)
		return
	}
	u.m.geoBusy = true
	u.addLog(tr("缺少 Geo 数据，开始下载（约 13 MB）…"), false)
	u.invalidate()
	go func() {
		derr := core.DownloadGeoData(u.ctx, dir, func(line string) {
			u.sync(func() { u.addLog(line, false) })
		})
		u.sync(func() {
			u.m.geoBusy = false
			if derr != nil {
				u.addLog(tr("Geo 数据下载失败: ")+derr.Error(), true)
				u.invalidate()
				return
			}
			u.addLog(tr("Geo 数据就绪，已启用「绕过大陆」"), false)
			u.mutateSettings(func(s *clientSettings) { s.ProxyMode = modeBypass })
		})
	}()
}

// gotoPage switches pages. The start page is always Home, so nothing about the
// current page is persisted.
func (u *natUI) gotoPage(page string) {
	switch page {
	case pageHome, pageNodes, pageRules, pageSettings, pageAbout, pageLogs:
	default:
		return
	}
	// Stamp a real page change so the shell slides the new page in; re-selecting
	// the current page is a no-op and must not replay the transition.
	if page != u.m.page {
		u.pageAnimAt = time.Now()
	}
	u.m.page = page
	u.m.scroll = 0
	u.m.dropOpen = ""
	if page != pageSettings {
		u.m.setScroll = 0
	}
	u.invalidate()
}

func (u *natUI) cycleTheme() {
	next := themeDark
	switch u.m.set.Theme {
	case themeLight:
		next = themeDark
	case themeDark:
		next = themeAuto
	default:
		next = themeLight
	}
	u.mutateSettings(func(s *clientSettings) { s.Theme = next })
	u.applyTheme()
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

func (u *natUI) actionSetting(id string) {
	// Toggles first: one line each, and they all behave the same way.
	toggle := func(fn func(*clientSettings)) {
		u.mutateSettings(fn)
	}
	switch id {
	case "set:allowlan":
		toggle(func(s *clientSettings) { s.AllowLan = !s.AllowLan })
		// Widen the firewall the moment sharing is switched on, not at connect
		// time: Windows prompts for any non-loopback listener, and that prompt
		// arriving mid-connect is what made the core look like it was asking for
		// network permission out of nowhere. Doing it here also means the user
		// sees the one UAC consent while they are already in a settings dialog.
		if u.m.set.AllowLan {
			port := u.m.set.MixedPort
			coreBin := coreBinaryPath(options{})
			go func() {
				err := core.EnsureLanFirewall(port, coreBin)
				u.sync(func() {
					if err != nil {
						u.addLog(trf("局域网共享：未能自动放行端口 %d（%v）。若其它设备连不上，请在 Windows 防火墙里允许 mihomo.exe", port, err), true)
					} else {
						u.addLog(trf("局域网共享已开启：端口 %d 与内核已放行，其它设备填 %s:%d", port, u.m.localLan, port), false)
					}
					u.invalidate()
				})
			}()
		} else {
			u.addLog(tr("局域网共享已关闭：监听回到 127.0.0.1"), false)
		}
		return
	case "set:autostart":
		next := !u.m.set.AutoStart
		if err := setAutoStart(next); err != nil {
			u.addLog(tr("写入启动项失败")+": "+err.Error(), true)
			return
		}
		u.mutateSettings(func(s *clientSettings) { s.AutoStart = next })
		return
	case "set:autoconnect":
		toggle(func(s *clientSettings) { s.AutoConnect = !s.AutoConnect })
		return
	case "set:availability":
		// The gate lives in the connect pipeline (startAndVerify), so the live
		// session is untouched until the next connect — which is why this says
		// so in the log instead of pretending it applied immediately.
		toggle(func(s *clientSettings) { s.AvailabilityCheck = !s.AvailabilityCheck })
		if u.m.set.AvailabilityCheck {
			u.addLog(tr("已开启可用性验证：过慢的节点会被跳过"), false)
		} else {
			u.addLog(tr("已关闭可用性验证：下次连接将直接连接"), false)
		}
		return
	case "set:dataguard":
		// The guardian is launched per session (ensureSessionGuard at connect
		// time), so a live session keeps the guard it already has until the
		// next connect — the log says so rather than pretending it applied at
		// once.
		toggle(func(s *clientSettings) { s.AutoRepair = !s.AutoRepair })
		if u.m.set.AutoRepair {
			u.addLog(tr("已开启数据面守护：下次连接后将持续探测并自动修复"), false)
		} else {
			u.addLog(tr("已关闭数据面守护：下次连接后将不再守护，数据面失效需手动重连"), false)
		}
		return
	}

	if v, ok := strings.CutPrefix(id, "pickcapture:"); ok {
		u.m.dropOpen = ""
		if v != captureProxy && v != captureTun {
			return
		}
		if v == captureTun && !core.IsElevated() {
			// Stored first: the choice survives either answer, so a user who
			// relaunches later gets the mode they picked.
			toggle(func(s *clientSettings) { s.CaptureMode = v })
			if u.confirm(tr("TUN 模式需要管理员权限"),
				tr("现在以管理员身份重启程序？重启后 TUN 模式即可生效。")) {
				if err := relaunchElevated(); err != nil {
					u.addLog(tr("以管理员身份重启失败: ")+err.Error(), true)
				} else {
					u.quitNow()
				}
			} else {
				u.addLog(tr("TUN 模式需要管理员权限：设置已保存，以管理员身份重新运行后生效"), true)
			}
			return
		}
		toggle(func(s *clientSettings) { s.CaptureMode = v })
		return
	}

	// Text fields: the canvas has no edit control, so every one of them opens
	// the same modal input box. Each setter validates its own input and returns
	// an error instead of silently keeping the old value.
	type field struct {
		hint  string
		value string
		set   func(*clientSettings, string) error
	}
	port := func(n int, other int, label string) error {
		if n < 1 || n > 65535 {
			return fmt.Errorf("%s", trf("%s 必须在 1-65535 之间", label))
		}
		if n == other {
			return fmt.Errorf("%s", trf("%s 不能与另一个端口相同", label))
		}
		return nil
	}
	fields := map[string]field{
		"set:lanuser": {tr("局域网共享的登录用户名；与密码同时填写才生效，留空表示不验证"),
			u.m.set.LanUser, func(s *clientSettings, v string) error {
				s.LanUser = strings.TrimSpace(v)
				return nil
			}},
		"set:lanpass": {tr("局域网共享的登录密码；与用户名同时填写才生效，留空表示不验证"),
			u.m.set.LanPass, func(s *clientSettings, v string) error {
				s.LanPass = strings.TrimSpace(v)
				return nil
			}},
		"set:mixedport": {tr("请输入 1-65535 之间的端口号"), itoa(int64(u.m.set.MixedPort)), func(s *clientSettings, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s", tr("端口必须是数字"))
			}
			if err := port(n, s.APIPort, tr("本地代理端口")); err != nil {
				return err
			}
			s.MixedPort = n
			return nil
		}},
		"set:domdirect": {tr("逗号分隔的域名后缀，如 example.com；留空不启用"), u.m.set.DomainDirect, func(s *clientSettings, v string) error {
			s.DomainDirect = strings.TrimSpace(v)
			return nil
		}},
		"set:domproxy": {tr("逗号分隔的域名后缀，如 example.com；留空不启用"), u.m.set.DomainProxy, func(s *clientSettings, v string) error {
			s.DomainProxy = strings.TrimSpace(v)
			return nil
		}},
		"set:picknode": {tr("端点，如 188.114.97.198:4500（留空 = 自动）"), u.m.pin, func(s *clientSettings, v string) error {
			v = strings.TrimSpace(v)
			if v != "" && !validPinEndpoint(v) {
				return fmt.Errorf("%s", tr("节点格式应为 IP 或 IP:端口"))
			}
			s.Pin = v
			return nil
		}},
	}
	f, ok := fields[id]
	if !ok {
		return
	}
	v, ok := u.promptTextCheck(tr(fieldTitle(id)), f.hint, f.value, func(raw string) error {
		// Validate against a copy: a rejected value must leave the live
		// settings untouched while the dialog stays open.
		tmp := u.m.set
		return f.set(&tmp, strings.TrimSpace(raw))
	})
	if !ok {
		return
	}
	label, val := fieldTitle(id), strings.TrimSpace(v)
	// Every outcome is reported on screen, not only in the log: the field may be
	// scrolled out of view, and "I clicked OK and nothing happened" is the worst
	// answer a settings dialog can give.
	if err := f.set(&u.m.set, val); err != nil {
		u.addLog(err.Error(), true)
		u.infoDialog(tr("设置未保存"), err.Error(), []string{trf("输入：%s", val)})
		return
	}
	if err := saveSettings(u.m.set); err != nil {
		u.addLog(tr("设置保存失败: ")+err.Error(), true)
		u.infoDialog(tr("设置未保存"), tr("设置保存失败: ")+err.Error(), nil)
		return
	}
	if u.m.connected {
		u.m.setDirty = true
	}
	u.addLog(tr("已保存：")+label+" = "+val, false)
	// Repaint first: the page should already show the new value when the user
	// dismisses the confirmation.
	u.invalidate()
	u.infoDialog(tr("设置已保存"), label+" = "+val, []string{tr("重新连接后生效")})
	if id == "set:picknode" {
		// normalize may have cleared a pin that does not match the family.
		if trimmed := strings.TrimSpace(v); trimmed != "" && u.m.set.Pin == "" {
			u.addLog(tr("该节点与当前地址族不匹配，已清除"), true)
		}
		u.m.pin = u.m.set.Pin
	}
	u.invalidate()
}

// validPinEndpoint accepts a bare IP or an IP:port endpoint.
func validPinEndpoint(v string) bool {
	if _, err := netip.ParseAddr(strings.Trim(v, "[]")); err == nil {
		return true
	}
	host, port, err := net.SplitHostPort(v)
	if err != nil {
		return false
	}
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func fieldTitle(id string) string {
	switch id {
	case "set:mixedport":
		return "本地代理端口"
	case "set:domdirect":
		return "直连域名"
	case "set:domproxy":
		return "代理域名"
	case "set:lanuser":
		return "共享认证用户名"
	case "set:lanpass":
		return "共享认证密码"
	default:
		return "默认节点"
	}
}

// syncSessionFromSettings pulls the session-shaped fields back out of the
// preference file (after a reset, or after loading a different language).
func (u *natUI) syncSessionFromSettings() {
	s := u.m.set
	u.m.proto = s.Proto
	u.m.pin = s.Pin
	u.m.psiphon = s.Psiphon
	u.m.openvpn = s.OpenVPN
	u.m.outerProto = s.OuterProto
	u.m.innerProto = s.InnerProto
	u.invalidate()
}

// ---------------------------------------------------------------------------
// About
// ---------------------------------------------------------------------------

func (u *natUI) actionAbout(id string) {
	switch id {
	case "about:copydiag":
		if err := setClipboardText(u.diagText()); err != nil {
			u.addLog(tr("复制失败")+": "+err.Error(), true)
		} else {
			u.addLog(tr("诊断信息已复制到剪贴板"), false)
		}
	case "about:opendata":
		// Still used by the Logs page's diagnostics tab.
		u.openDataDir()
	}
	// The link card's rows are upstream projects; opening one is the same
	// action whatever the row, so they share the branch below.
	if url, ok := aboutLinkURL(id); ok {
		openURL(url)
	}
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------
