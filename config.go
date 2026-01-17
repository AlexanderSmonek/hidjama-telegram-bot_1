package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Token          string
	Admins         []int64
	Timezone       string
	Debug          bool
	DevPassword    string
	SupabaseURL    string
	SupabaseKey    string
}

func loadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		// No .env file, continue
	}

	cfg := &Config{
		Token:          os.Getenv("BOT_TOKEN"),
		Timezone:       "Europe/Moscow",
		DevPassword:    getEnv("DEV_PASSWORD", "4116"),
		Debug:          false,
		SupabaseURL:    os.Getenv("SUPABASE_URL"),
		SupabaseKey:    os.Getenv("SUPABASE_KEY"),
	}

	// Load admins
	adminsStr := os.Getenv("ADMINS")
	if adminsStr != "" {
		parts := strings.Split(adminsStr, ",")
		for _, p := range parts {
			if id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
				cfg.Admins = append(cfg.Admins, id)
			}
		}
	} else {
		cfg.Admins = []int64{348038520, 1831673006, 7401260307, 6064116707}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
