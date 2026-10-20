package store

import "testing"

func TestPasswordOK(t *testing.T) {
	if err := PasswordOK("Abcd123!"); err != nil {
		t.Fatal(err)
	}
	if err := PasswordOK("abcd123!"); err == nil {
		t.Fatal("missing upper")
	}
	if err := PasswordOK("ABCD123!"); err == nil {
		t.Fatal("missing lower")
	}
	if err := PasswordOK("Abcdefg!"); err == nil {
		t.Fatal("missing digit")
	}
	if err := PasswordOK("Abcd1234"); err == nil {
		t.Fatal("missing symbol")
	}
	if err := PasswordOK("Aa1!"); err == nil {
		t.Fatal("too short")
	}
}

func TestUsernameOK(t *testing.T) {
	name, key, err := UsernameOK(" 花园 ")
	if err != nil || name != "花园" || key != "花园" {
		t.Fatalf("%q %q %v", name, key, err)
	}
	if _, _, err := UsernameOK("a"); err == nil {
		t.Fatal("too short")
	}
	if _, _, err := UsernameOK("a b"); err == nil {
		t.Fatal("space")
	}
}
