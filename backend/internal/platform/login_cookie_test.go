package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecryptChromeCookie(t *testing.T) {
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	db := filepath.Join(dir, "forking-paths", "login-psn", "Default", "Cookies")
	if _, err := os.Stat(db); err != nil {
		t.Skip(err)
	}
	key, err := chromeSafeKey()
	if err != nil {
		t.Fatal(err)
	}
	value, err := profileCookie(filepath.Join(dir, "forking-paths", "login-psn"), key, "country", "")
	if err != nil {
		t.Fatal(err)
	}
	if value == "" {
		t.Fatal("empty cookie")
	}
}
