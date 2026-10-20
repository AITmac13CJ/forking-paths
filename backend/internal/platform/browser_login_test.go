package platform

import (
	"strings"
	"testing"
)

func TestBrowserLoginStaysOnOfficialSites(t *testing.T) {
	psn, err := BrowserLogin("psn")
	if err != nil || !strings.Contains(psn, "https://ca.account.sony.com/api/v1/ssocookie") || strings.Contains(psn, "127.0.0.1") {
		t.Fatalf("psn url: %v %s", err, psn)
	}
	sw, err := BrowserLogin("switch")
	if err != nil || !strings.HasPrefix(sw, "https://accounts.nintendo.com/connect/1.0.0/authorize?") || strings.Contains(sw, "127.0.0.1") {
		t.Fatalf("switch url: %v %s", err, sw)
	}
}

func TestParseNPSSO(t *testing.T) {
	token := "abcdefghijklmnopqrstuvwxyz012345"
	if got := parseNPSSO(token); got != token {
		t.Fatalf("raw: %q", got)
	}
	if got := parseNPSSO("{\"npsso\":\"" + token + "\"}"); got != token {
		t.Fatalf("json: %q", got)
	}
	if got := parseNPSSO("页面 {\"npsso\": \"" + token + "\"}"); got != token {
		t.Fatalf("wrapped: %q", got)
	}
	if parseNPSSO("太短") != "" || parseNPSSO("not a token with spaces "+token) != "" {
		t.Fatal("rejected invalid text")
	}
}

func TestPastedParams(t *testing.T) {
	cases := []struct {
		raw   string
		key   string
		value string
	}{
		{"https://login.live.com/oauth20_desktop.srf?code=live-code", "code", "live-code"},
		{"com.scee.psxandroid.scecompcall://redirect?code=psn-code", "code", "psn-code"},
		{"npf5c38e31cd085304b://auth#session_token_code=sw-code&state=sw-state", "session_token_code", "sw-code"},
		{"请看这个 npf5c38e31cd085304b://auth#session_token_code=sw-code&state=sw-state", "state", "sw-state"},
	}
	for _, tc := range cases {
		got := pastedParams(tc.raw).Get(tc.key)
		if got != tc.value {
			t.Fatalf("%s: got %q", tc.key, got)
		}
	}
}
