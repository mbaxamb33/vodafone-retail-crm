// Package config reads runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env         string // development or production
	ListenAddr  string
	AppOrigin   string
	DatabaseURL string
	AutoMigrate bool
	SessionTTL  time.Duration
	LogLevel    slog.Level
	Features    []string
}

func get(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func Load() (Config, error) {
	c := Config{
		Env:         get("APP_ENV", "development"),
		ListenAddr:  get("LISTEN_ADDR", "127.0.0.1:8080"),
		AppOrigin:   strings.TrimRight(get("APP_ORIGIN", "http://127.0.0.1:5173"), "/"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	var errs []error
	if c.Env != "development" && c.Env != "production" {
		errs = append(errs, errors.New("APP_ENV must be development or production"))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	var err error
	if c.AutoMigrate, err = strconv.ParseBool(get("AUTO_MIGRATE", "true")); err != nil {
		errs = append(errs, errors.New("AUTO_MIGRATE must be true or false"))
	}
	if c.SessionTTL, err = time.ParseDuration(get("SESSION_TTL", "8h")); err != nil || c.SessionTTL < time.Minute {
		errs = append(errs, errors.New("SESSION_TTL must be a duration of at least 1m"))
	}
	if err := c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, errors.New("LOG_LEVEL must be debug, info, warn or error"))
	}
	for _, f := range strings.Split(os.Getenv("FEATURES"), ",") {
		if f = strings.TrimSpace(f); f != "" {
			c.Features = append(c.Features, f)
		}
	}
	if c.Env == "production" && !strings.HasPrefix(c.AppOrigin, "https://") {
		errs = append(errs, fmt.Errorf("APP_ORIGIN must use https in production"))
	}
	return c, errors.Join(errs...)
}
