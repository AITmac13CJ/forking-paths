package platform

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestSteamAccessUsable(t *testing.T) {
	if SteamAccessUsable("") {
		t.Fatal("empty token")
	}
	if SteamAccessUsable("not-a-jwt") {
		t.Fatal("unparsed token")
	}
	expired := steamJWT(time.Now().Add(-time.Hour))
	if SteamAccessUsable(expired) {
		t.Fatal("expired token")
	}
	fresh := steamJWT(time.Now().Add(2 * time.Hour))
	if !SteamAccessUsable(fresh) {
		t.Fatal("fresh token")
	}
}

func steamJWT(exp time.Time) string {
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	body := base64.RawURLEncoding.EncodeToString(payload)
	return head + "." + body + ".x"
}
