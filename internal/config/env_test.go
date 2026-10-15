package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ibobrov/sharetrip_notification/internal/config"
)

// These tests mutate process environment and must remain sequential.
func baseline(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"HTTP_HOST", "HTTP_PORT", "DB_HOST", "DB_PORT", "DB_NAME",
		"DB_USER", "DB_PASSWORD", "DB_SSLMODE",
	} {
		unset(t, name)
	}
	t.Setenv("DB_USER", "notification_test")
	t.Setenv("DB_PASSWORD", " test-secret:@/ ?# ")
}

func unset(t *testing.T, name string) {
	t.Helper()
	// Setenv registers restoration of the original environment with Cleanup.
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
}

//nolint:paralleltest // baseline calls t.Setenv, which is incompatible with parallel tests.
func TestLoadDefaults(t *testing.T) {
	baseline(t)
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		HTTP: config.HTTPConfig{Host: "127.0.0.1", Port: 8081},
		DB: config.DBConfig{
			Host: "localhost", Port: 6544, Name: "sharetrip_notification",
			User: "notification_test", Password: " test-secret:@/ ?# ", SSLMode: "disable",
		},
	}
	if got != want {
		t.Fatal("defaults or unchanged credential values do not match")
	}
}

//nolint:paralleltest // This test changes process environment with t.Setenv and must run sequentially.
func TestLoadExplicitValues(t *testing.T) {
	baseline(t)
	for name, v := range map[string]string{
		"HTTP_HOST": "::1", "HTTP_PORT": "9000", "DB_HOST": "postgres",
		"DB_PORT": "5432", "DB_NAME": "notifications_test", "DB_USER": "explicit_user",
		"DB_PASSWORD": "explicit-secret", "DB_SSLMODE": "verify-full",
	} {
		t.Setenv(name, v)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		HTTP: config.HTTPConfig{Host: "::1", Port: 9000},
		DB: config.DBConfig{
			Host: "postgres", Port: 5432, Name: "notifications_test",
			User: "explicit_user", Password: "explicit-secret", SSLMode: "verify-full",
		},
	}
	if got != want {
		t.Fatal("explicit environment values do not match")
	}
}

func TestLoadRequiredValues(t *testing.T) {
	for _, name := range []string{"DB_USER", "DB_PASSWORD"} {
		t.Run(name+"/unset", func(t *testing.T) {
			baseline(t)
			unset(t, name)
			cfg, err := config.Load()
			assertInvalid(t, name, cfg, err)
		})
	}
	for _, name := range []string{"HTTP_HOST", "DB_HOST", "DB_NAME", "DB_USER", "DB_PASSWORD"} {
		for _, v := range []string{"", " \t\n"} {
			t.Run(name+"/blank/"+v, func(t *testing.T) {
				baseline(t)
				t.Setenv(name, v)
				cfg, err := config.Load()
				assertInvalid(t, name, cfg, err)
			})
		}
	}
}

func TestLoadPorts(t *testing.T) {
	for _, name := range []string{"HTTP_PORT", "DB_PORT"} {
		for _, v := range []string{"", "abc", "0", "-1", "65536", "999999999999999999999", "1.5", " 5432 "} {
			t.Run(name+"/invalid/"+v, func(t *testing.T) {
				baseline(t)
				t.Setenv(name, v)
				cfg, err := config.Load()
				assertInvalid(t, name, cfg, err)
			})
		}
		for _, tc := range []struct {
			value string
			want  int
		}{{"1", 1}, {"65535", 65535}} {
			t.Run(name+"/valid/"+tc.value, func(t *testing.T) {
				baseline(t)
				t.Setenv(name, tc.value)
				got, err := config.Load()
				if err != nil {
					t.Fatal(err)
				}
				actual := got.HTTP.Port
				if name == "DB_PORT" {
					actual = got.DB.Port
				}
				if actual != tc.want {
					t.Fatalf("port = %d, want %d", actual, tc.want)
				}
			})
		}
	}
}

func TestLoadSSLMode(t *testing.T) {
	for _, mode := range []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"} {
		t.Run(mode, func(t *testing.T) {
			baseline(t)
			t.Setenv("DB_SSLMODE", mode)
			got, err := config.Load()
			if err != nil || got.DB.SSLMode != mode {
				t.Fatalf("SSL mode was not accepted: %v", err)
			}
		})
	}
	for _, mode := range []string{"", " ", "invalid"} {
		t.Run("invalid/"+mode, func(t *testing.T) {
			baseline(t)
			t.Setenv("DB_SSLMODE", mode)
			cfg, err := config.Load()
			assertInvalid(t, "DB_SSLMODE", cfg, err)
		})
	}
}

func TestLoadErrorsDoNotExposeValues(t *testing.T) {
	const sensitive = "postgres://user:secret@localhost/notifications"
	for _, name := range []string{"HTTP_PORT", "DB_PORT", "DB_SSLMODE"} {
		t.Run(name, func(t *testing.T) {
			baseline(t)
			t.Setenv("DB_PASSWORD", sensitive)
			t.Setenv(name, sensitive)
			cfg, err := config.Load()
			assertInvalid(t, name, cfg, err)
			if strings.Contains(err.Error(), sensitive) || strings.Contains(err.Error(), "secret") {
				t.Fatal("configuration error exposes an input value")
			}
		})
	}
}

func assertInvalid(t *testing.T, name string, cfg config.Config, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("error does not identify %s", name)
	}
	if cfg != (config.Config{}) {
		t.Fatal("invalid configuration must not return partial settings")
	}
}
