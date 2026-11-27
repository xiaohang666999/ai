//go:build windows

package main

import "testing"

// TestExitAvailableGates pins the settings page's one availability gate for the
// exit switches and their region pickers: an exit must be stackable at all
// (no chained protocol outside subscription mode) and the tunnel must not be
// v6-only. The picker is NOT tied to the switch itself — see the paint site in
// paintSetExit, which passes this gate rather than "available && exit on".
func TestExitAvailableGates(t *testing.T) {
	cases := []struct {
		name   string
		proto  string
		mode   string
		family string
		want   bool
	}{
		{"warp wg v4", protoWG, "", famV4, true},
		{"warp awg dual stack", protoAWG, "", famBoth, true},
		{"subscription ignores the leftover proto", protoWWarp, "subscription", famV4, true},
		{"chained protocol in warp mode", protoWWarp, "", famV4, false},
		{"masque-in-masque in warp mode", protoMASQUEMASQUE, "", famV4, false},
		{"v6-only tunnel", protoAWG, "", famV6, false},
		{"v6-only subscription", protoWG, "subscription", famV6, false},
	}
	for _, c := range cases {
		u := &natUI{m: &natModel{
			proto: c.proto,
			set:   clientSettings{ConnMode: c.mode, IPFamily: c.family},
		}}
		if got := u.exitAvailable(); got != c.want {
			t.Fatalf("%s: exitAvailable = %v, want %v", c.name, got, c.want)
		}
		// No core probed yet means protoOK is optimistic, so the VPN Gate
		// gate mirrors the base one here.
		if got := u.vpnExitAvailable(); got != c.want {
			t.Fatalf("%s: vpnExitAvailable = %v, want %v", c.name, got, c.want)
		}
	}

	// The core capability half: an installed core whose probe rejects the
	// openvpn outbound closes the VPN Gate switch and picker but must leave the
	// Psiphon exit alone.
	u := &natUI{m: &natModel{
		proto:         protoAWG,
		coreInstalled: true,
		capab:         map[string]bool{protoAWG: true, probeOpenVPNProto: false},
		set:           clientSettings{IPFamily: famV4},
	}}
	if !u.exitAvailable() {
		t.Fatal("exitAvailable must not depend on the VPN Gate capability")
	}
	if u.vpnExitAvailable() {
		t.Fatal("a core without the openvpn outbound must close the VPN Gate gate")
	}
	u.m.capab[probeOpenVPNProto] = true
	if !u.vpnExitAvailable() {
		t.Fatal("a core with the openvpn outbound must open the VPN Gate gate")
	}
}

// TestExitRegionChangeOnlyReconnectsLiveExit pins the action rule behind the
// pickers: changing a saved region re-applies by reconnecting only when the
// running session actually exits through that exit. With the exit off (the
// common case now that the picker stays editable) saving is the whole job and
// the session must be left alone.
func TestExitRegionChangeOnlyReconnectsLiveExit(t *testing.T) {
	u := &natUI{m: &natModel{
		connected: true,
		proto:     protoWG,
		set:       clientSettings{IPFamily: famV4},
	}}
	if u.psiphonExitActive() || u.openvpnExitActive() {
		t.Fatal("a session with both exits off must not be reconnected by a region change")
	}

	u.m.psiphon = true
	if !u.psiphonExitActive() {
		t.Fatal("a Psiphon session must re-apply a region change")
	}
	if u.openvpnExitActive() {
		t.Fatal("OpenVPN must not be live while Psiphon is the exit")
	}

	u.m.psiphon, u.m.openvpn = false, true
	if !u.openvpnExitActive() {
		t.Fatal("a VPN Gate session must re-apply a region change")
	}
	if u.psiphonExitActive() {
		t.Fatal("Psiphon must not be live while OpenVPN is the exit")
	}

	// Idle sessions never reconnect, whatever the switches remember.
	u.m.connected = false
	if u.openvpnExitActive() || u.psiphonExitActive() {
		t.Fatal("a disconnected client must not reconnect on a region change")
	}

	// A v6-only tunnel cannot stack the exit, so a saved region cannot reach a
	// running session either.
	u.m.connected, u.m.openvpn = true, true
	u.m.set.IPFamily = famV6
	if u.openvpnExitActive() {
		t.Fatal("an exit that cannot stack on this tunnel must not be re-applied")
	}
}
