package config

import "os"

type Config struct {
	DatabaseURL string
	Addr        string
}

func Load() Config {
	port := getenv("PORT", "8080")
	return Config{
		DatabaseURL: getenv("DATABASE_URL", "postgres://gamelog:gamelog@localhost:5433/gamelog?sslmode=disable"),
		Addr:        ":" + port,
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
