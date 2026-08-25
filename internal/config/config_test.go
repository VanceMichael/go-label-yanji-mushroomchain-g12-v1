package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load defaults: %v", err)
	}
	if cfg.Address != ":8080" {
		t.Fatalf("address=%q", cfg.Address)
	}
	if cfg.DatabasePath != "mushroomchain.db" {
		t.Fatalf("database=%q", cfg.DatabasePath)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("session ttl=%s", cfg.SessionTTL)
	}
	if cfg.WorkerInterval != time.Second {
		t.Fatalf("worker interval=%s", cfg.WorkerInterval)
	}
	if cfg.LeaseDuration != 30*time.Second {
		t.Fatalf("lease=%s", cfg.LeaseDuration)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("shutdown=%s", cfg.ShutdownTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("log level=%q", cfg.LogLevel)
	}
}

func TestLoadOverridesEverySetting(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("DATABASE_PATH", t.TempDir()+"/custom.db")
	t.Setenv("SESSION_TTL", "45m")
	t.Setenv("WORKER_INTERVAL", "250ms")
	t.Setenv("WORKER_LEASE", "5s")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("LOG_LEVEL", "WARN")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load overrides: %v", err)
	}
	if cfg.Address != "127.0.0.1:9090" {
		t.Fatalf("address=%q", cfg.Address)
	}
	if cfg.SessionTTL != 45*time.Minute {
		t.Fatalf("ttl=%s", cfg.SessionTTL)
	}
	if cfg.WorkerInterval != 250*time.Millisecond {
		t.Fatalf("interval=%s", cfg.WorkerInterval)
	}
	if cfg.LeaseDuration != 5*time.Second {
		t.Fatalf("lease=%s", cfg.LeaseDuration)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("shutdown=%s", cfg.ShutdownTimeout)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("level=%q", cfg.LogLevel)
	}
}

func TestLoadAcceptsIntegerSeconds(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("SESSION_TTL", "3600")
	t.Setenv("WORKER_INTERVAL", "2")
	t.Setenv("WORKER_LEASE", "10")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionTTL != time.Hour || cfg.WorkerInterval != 2*time.Second || cfg.LeaseDuration != 10*time.Second {
		t.Fatalf("durations=%+v", cfg)
	}
}

func TestLoadRejectsMalformedDurations(t *testing.T) {
	variables := []string{"SESSION_TTL", "WORKER_INTERVAL", "WORKER_LEASE", "SHUTDOWN_TIMEOUT"}
	for _, name := range variables {
		t.Run(name, func(t *testing.T) {
			clearConfigEnvironment(t)
			t.Setenv(name, "not-a-duration")
			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted malformed %s", name)
			}
		})
	}
}

func TestValidateRejectsEmptyAddressAndDatabase(t *testing.T) {
	cfg := validConfig()
	cfg.Address = ""
	cfg.DatabasePath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate succeeded")
	}
	if count := joinedErrorCount(err); count < 2 {
		t.Fatalf("joined errors=%d: %v", count, err)
	}
}

func TestValidateRejectsNonPositiveDurations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{{"session", func(c *Config) { c.SessionTTL = 0 }}, {"worker", func(c *Config) { c.WorkerInterval = -time.Second }}, {"shutdown", func(c *Config) { c.ShutdownTimeout = 0 }}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate succeeded")
			}
		})
	}
}

func TestValidateRequiresLeaseLongerThanInterval(t *testing.T) {
	cfg := validConfig()
	cfg.WorkerInterval = 5 * time.Second
	cfg.LeaseDuration = 5 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("equal lease accepted")
	}
	cfg.LeaseDuration = 4 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("short lease accepted")
	}
	cfg.LeaseDuration = 6 * time.Second
	if err := cfg.Validate(); err != nil {
		t.Fatalf("long lease rejected: %v", err)
	}
}

func TestValidateLogLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		cfg := validConfig()
		cfg.LogLevel = level
		if err := cfg.Validate(); err != nil {
			t.Fatalf("level %s: %v", level, err)
		}
	}
	for _, level := range []string{"", "trace", "INFO", "warning"} {
		cfg := validConfig()
		cfg.LogLevel = level
		if err := cfg.Validate(); err == nil {
			t.Fatalf("level %q accepted", level)
		}
	}
}

func validConfig() Config {
	return Config{Address: ":8080", DatabasePath: "test.db", SessionTTL: time.Hour, WorkerInterval: time.Second, LeaseDuration: 10 * time.Second, ShutdownTimeout: time.Second, LogLevel: "info"}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"HTTP_ADDR", "DATABASE_PATH", "SESSION_TTL", "WORKER_INTERVAL", "WORKER_LEASE", "SHUTDOWN_TIMEOUT", "LOG_LEVEL"} {
		old, ok := os.LookupEnv(name)
		_ = os.Unsetenv(name)
		name := name
		if ok {
			t.Cleanup(func() { _ = os.Setenv(name, old) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(name) })
		}
	}
}

func joinedErrorCount(err error) int {
	type unwrapper interface{ Unwrap() []error }
	var joined unwrapper
	if !errors.As(err, &joined) {
		return 1
	}
	return len(joined.Unwrap())
}
