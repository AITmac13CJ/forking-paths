package platform

import "testing"

func TestParseSteamLogin(t *testing.T) {
	id, token, ok := parseSteamLogin("76561198000000000%7C%7Ctoken-value")
	if !ok || id != "76561198000000000" || token != "token-value" {
		t.Fatalf("%q %q %v", id, token, ok)
	}
	if _, _, ok := parseSteamLogin("short"); ok {
		t.Fatal("accepted short cookie")
	}
}

func TestApplyTrophies(t *testing.T) {
	var item Game
	applyTrophies(&item, 40, trophyCounts{Bronze: 3, Silver: 1}, trophyCounts{Bronze: 10, Silver: 4, Gold: 2, Platinum: 1})
	if item.Earned != 4 || item.Total != 17 || item.Complete || item.Platinum {
		t.Fatalf("%+v", item)
	}
	applyTrophies(&item, 100, trophyCounts{Bronze: 10, Silver: 4, Gold: 2, Platinum: 1}, trophyCounts{Bronze: 10, Silver: 4, Gold: 2, Platinum: 1})
	if !item.Complete || !item.Platinum || item.Earned != 17 {
		t.Fatalf("%+v", item)
	}
}
