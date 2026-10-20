package platform

import "testing"

func TestNormalizeOnlineID(t *testing.T) {
	id, ok := NormalizeOnlineID(" Garden_User-1 ")
	if !ok || id != "Garden_User-1" {
		t.Fatalf("got %q %v", id, ok)
	}
	if _, ok := NormalizeOnlineID("短"); ok {
		t.Fatal("accepted a short name")
	}
	if _, ok := NormalizeOnlineID("has space"); ok {
		t.Fatal("accepted spaces")
	}
}

func TestParsePSNCredKeepsOnlineIDSeparate(t *testing.T) {
	cred, ok := parsePSNCred(`{"online_id":"GardenUser"}`)
	if !ok || cred.OnlineID != "GardenUser" || cred.NPSSO != "" || cred.Access != "" {
		t.Fatalf("cred %+v ok %v", cred, ok)
	}
}

func TestNormalizeFriendCode(t *testing.T) {
	code, ok := NormalizeFriendCode("sw123456789012")
	if !ok || code != "SW-1234-5678-9012" {
		t.Fatalf("got %q %v", code, ok)
	}
	code, ok = NormalizeFriendCode("SW-1234-5678-9012")
	if !ok || code != "SW-1234-5678-9012" {
		t.Fatalf("dashed: %q %v", code, ok)
	}
	if _, ok := NormalizeFriendCode("SW-123"); ok {
		t.Fatal("accepted a short code")
	}
}
