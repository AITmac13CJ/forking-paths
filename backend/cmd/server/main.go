package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"forking-paths/internal/config"
	"forking-paths/internal/db"
	"forking-paths/internal/httpapi"
	"forking-paths/internal/store"
)

func main() {
	cfg := config.Load()
	pool, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	st := store.New(pool)
	if err := st.FailInterruptedJobs(ctx); err != nil {
		log.Fatal(err)
	}
	if err := st.ImportLegacy(ctx); err != nil {
		log.Fatal(err)
	}
	if os.Getenv("GARDEN_SEED") == "0" {
		if err := st.EnsureCatalog(ctx); err != nil {
			log.Fatal(err)
		}
	} else {
		n, err := st.Seed(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if n > 0 {
			log.Printf("已写入 %d 条记录", n)
		}
	}

	api := httpapi.New(st)
	handler := api
	if covers, err := httpapi.CoversDir(); err == nil {
		files := httpapi.ServeCovers(covers)
		prev := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/covers/") {
				files.ServeHTTP(w, r)
				return
			}
			prev.ServeHTTP(w, r)
		})
	}
	if dist := findDist(); dist != "" {
		log.Printf("同时提供前端静态文件 %s", dist)
		pages := spaHandler(dist)
		prev := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/covers/") {
				prev.ServeHTTP(w, r)
				return
			}
			pages.ServeHTTP(w, r)
		})
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("小径分岔的花园已开 http://localhost%s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func findDist() string {
	for _, p := range []string{"frontend/dist", "../frontend/dist"} {
		info, err := os.Stat(filepath.Join(p, "index.html"))
		if err == nil && !info.IsDir() {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

func spaHandler(dist string) http.Handler {
	index := filepath.Join(dist, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/")
		if rel == "" || rel == "." {
			http.ServeFile(w, r, index)
			return
		}
		full := filepath.Join(dist, rel)
		if !strings.HasPrefix(full, dist+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(full)
		if err == nil && !info.IsDir() {
			http.ServeFile(w, r, full)
			return
		}
		http.ServeFile(w, r, index)
	})
}
