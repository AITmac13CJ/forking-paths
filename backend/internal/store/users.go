package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const sessionDays = 30

type User struct {
	ID       int
	Username string
}

func (s *Store) Register(ctx context.Context, username, password, recovery string) (User, string, error) {
	name, key, err := UsernameOK(username)
	if err != nil {
		return User{}, "", err
	}
	if err := PasswordOK(password); err != nil {
		return User{}, "", err
	}
	recovery = strings.TrimSpace(recovery)
	if err := RecoveryOK(recovery); err != nil {
		return User{}, "", err
	}
	passHash, err := hashSecret(password)
	if err != nil {
		return User{}, "", err
	}
	recHash, err := hashSecret(recovery)
	if err != nil {
		return User{}, "", err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback(ctx)

	var user User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (username, username_key, password_hash, recovery_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username
	`, name, key, passHash, recHash).Scan(&user.ID, &user.Username)
	if err != nil {
		if strings.Contains(err.Error(), "users_username_key_key") || strings.Contains(err.Error(), "23505") {
			return User{}, "", &ValidationError{Msg: "这个账号已经有人用了"}
		}
		return User{}, "", err
	}
	var others int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM users WHERE id <> $1`, user.ID).Scan(&others); err != nil {
		return User{}, "", err
	}
	if others == 0 {
		if _, err := tx.Exec(ctx, `UPDATE entries SET user_id = $1 WHERE user_id IS NULL`, user.ID); err != nil {
			return User{}, "", err
		}
		if _, err := tx.Exec(ctx, `UPDATE platform_accounts SET user_id = $1 WHERE user_id IS NULL`, user.ID); err != nil {
			return User{}, "", err
		}
	}
	token, err := insertSession(ctx, tx, user.ID)
	if err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Store) Login(ctx context.Context, username, password string) (User, string, error) {
	_, key, err := UsernameOK(username)
	if err != nil {
		return User{}, "", &ValidationError{Msg: "账号或密码不对"}
	}
	var user User
	var hash string
	err = s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash FROM users WHERE username_key = $1
	`, key).Scan(&user.ID, &user.Username, &hash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !matchSecret(hash, password)) {
		return User{}, "", &ValidationError{Msg: "账号或密码不对"}
	}
	if err != nil {
		return User{}, "", err
	}
	token, err := insertSession(ctx, s.pool, user.ID)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Store) ResetPassword(ctx context.Context, username, recovery, password string) (User, string, error) {
	_, key, err := UsernameOK(username)
	if err != nil {
		return User{}, "", &ValidationError{Msg: "账号或找回暗号不对"}
	}
	recovery = strings.TrimSpace(recovery)
	if err := RecoveryOK(recovery); err != nil {
		return User{}, "", &ValidationError{Msg: "账号或找回暗号不对"}
	}
	if err := PasswordOK(password); err != nil {
		return User{}, "", err
	}
	var user User
	var recHash string
	err = s.pool.QueryRow(ctx, `
		SELECT id, username, recovery_hash FROM users WHERE username_key = $1
	`, key).Scan(&user.ID, &user.Username, &recHash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !matchSecret(recHash, recovery)) {
		return User{}, "", &ValidationError{Msg: "账号或找回暗号不对"}
	}
	if err != nil {
		return User{}, "", err
	}
	passHash, err := hashSecret(password)
	if err != nil {
		return User{}, "", err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, passHash, user.ID); err != nil {
		return User{}, "", err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, user.ID); err != nil {
		return User{}, "", err
	}
	token, err := insertSession(ctx, s.pool, user.ID)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Store) UserByToken(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthorized
	}
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()
	`, hashToken(token)).Scan(&user.ID, &user.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	return user, err
}

func (s *Store) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func insertSession(ctx context.Context, q execer, userID int) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	_, err := q.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, hashToken(token), userID, time.Now().Add(sessionDays*24*time.Hour))
	return token, err
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
