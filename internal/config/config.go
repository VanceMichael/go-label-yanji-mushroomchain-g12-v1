package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address         string
	DatabasePath    string
	SessionTTL      time.Duration
	WorkerInterval  time.Duration
	LeaseDuration   time.Duration
	ShutdownTimeout time.Duration
	LogLevel        string
}

func Load() (Config, error) {
	cfg := Config{
		Address:         env("HTTP_ADDR", ":8080"),
		DatabasePath:    env("DATABASE_PATH", "mushroomchain.db"),
		SessionTTL:      duration("SESSION_TTL", 12*time.Hour),
		WorkerInterval:  duration("WORKER_INTERVAL", time.Second),
		LeaseDuration:   duration("WORKER_LEASE", 30*time.Second),
		ShutdownTimeout: duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		LogLevel:        strings.ToLower(env("LOG_LEVEL", "info")),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []error
	if strings.TrimSpace(c.Address) == "" {
		problems = append(problems, errors.New("HTTP_ADDR is empty"))
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		problems = append(problems, errors.New("DATABASE_PATH is empty"))
	}
	if c.SessionTTL <= 0 {
		problems = append(problems, errors.New("SESSION_TTL must be positive"))
	}
	if c.WorkerInterval <= 0 {
		problems = append(problems, errors.New("WORKER_INTERVAL must be positive"))
	}
	if c.LeaseDuration <= c.WorkerInterval {
		problems = append(problems, errors.New("WORKER_LEASE must exceed WORKER_INTERVAL"))
	}
	if c.ShutdownTimeout <= 0 {
		problems = append(problems, errors.New("SHUTDOWN_TIMEOUT must be positive"))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, fmt.Errorf("unsupported LOG_LEVEL %q", c.LogLevel))
	}
	return errors.Join(problems...)
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func duration(name string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, numberErr := strconv.Atoi(value)
	if numberErr == nil {
		return time.Duration(seconds) * time.Second
	}
	return -1
}
