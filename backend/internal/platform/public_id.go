package platform

import (
	"context"
	"strings"
	"time"
	"unicode"
)

func NormalizeOnlineID(raw string) (string, bool) {
	id := strings.TrimSpace(raw)
	if len(id) < 3 || len(id) > 16 {
		return "", false
	}
	for _, r := range id {
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_') {
			return "", false
		}
	}
	return id, true
}

func NormalizeFriendCode(raw string) (string, bool) {
	compact := strings.ToUpper(strings.TrimSpace(raw))
	compact = strings.ReplaceAll(compact, "-", "")
	compact = strings.ReplaceAll(compact, " ", "")
	compact = strings.TrimPrefix(compact, "SW")
	if len(compact) != 12 {
		return "", false
	}
	for _, r := range compact {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return "SW-" + compact[0:4] + "-" + compact[4:8] + "-" + compact[8:12], true
}

func PSNAccessFromSecret(ctx context.Context, client *Client, secret string) string {
	cred, ok := parsePSNCred(secret)
	if !ok {
		return ""
	}
	if cred.Access != "" && !jwtExpiring(cred.Access, time.Minute) {
		return cred.Access
	}
	if cred.NPSSO == "" {
		return ""
	}
	access, err := client.psnToken(ctx, cred.NPSSO)
	if err != nil {
		return ""
	}
	return access
}
