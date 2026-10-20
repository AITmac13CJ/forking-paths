package store

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

func PasswordOK(password string) error {
	if len(password) > 72 {
		return &ValidationError{Msg: "密码最长 72 个字节"}
	}
	if utf8.RuneCountInString(password) < 8 {
		return &ValidationError{Msg: "密码至少 8 位，并包含大写、小写、数字和符号"}
	}
	var upper, lower, digit, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsLetter(r):
		default:
			symbol = true
		}
	}
	if !upper || !lower || !digit || !symbol {
		return &ValidationError{Msg: "密码至少 8 位，并包含大写、小写、数字和符号"}
	}
	return nil
}

func UsernameOK(name string) (string, string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 2 || n > 24 {
		return "", "", &ValidationError{Msg: "账号需要 2 到 24 个字"}
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", "", &ValidationError{Msg: "账号里不要有空格"}
		}
	}
	return name, strings.ToLower(name), nil
}

func RecoveryOK(phrase string) error {
	phrase = strings.TrimSpace(phrase)
	if utf8.RuneCountInString(phrase) < 4 || utf8.RuneCountInString(phrase) > 40 {
		return &ValidationError{Msg: "找回暗号需要 4 到 40 个字"}
	}
	return nil
}

func hashSecret(plain string) (string, error) {
	sum, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(sum), nil
}

func matchSecret(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
