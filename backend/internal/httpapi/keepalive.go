package httpapi

import (
	"context"
	"log"
	"time"

	"forking-paths/internal/platform"
	"forking-paths/internal/store"
)

func (s *Server) keepLogins() {
	s.renewLogins()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.renewLogins()
	}
}

func (s *Server) renewLogins() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	accounts, err := s.store.StoredAccounts(ctx)
	if err != nil {
		log.Printf("登录续期没有开始: %v", err)
		return
	}
	client := platform.NewClient()
	for _, item := range accounts {
		next, changed, err := client.Renew(ctx, item.Provider, item.Secret)
		if err != nil {
			log.Printf("%s 登录还不能自动续上", item.Provider)
			continue
		}
		if !changed {
			continue
		}
		save, cancel := context.WithTimeout(store.WithUser(context.Background(), item.UserID), 10*time.Second)
		err = s.store.UpdateSecret(save, item.Provider, next)
		cancel()
		if err != nil {
			log.Printf("%s 登录续期没有写下", item.Provider)
			continue
		}
		log.Printf("已续上 %s 登录", item.Provider)
	}
}
