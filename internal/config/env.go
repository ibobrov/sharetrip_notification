package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTP HTTPConfig
	DB   DBConfig
}

type HTTPConfig struct {
	Host string
	Port int
}

type DBConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

// Load reads process environment without opening connections or loading files.
// Defaults apply only to unset variables; explicit empty values are invalid.
func Load() (Config, error) {
	cfg := Config{
		HTTP: HTTPConfig{Host: value("HTTP_HOST", "127.0.0.1")},
		DB: DBConfig{
			Host:     value("DB_HOST", "localhost"),
			Name:     value("DB_NAME", "sharetrip_notification"),
			User:     value("DB_USER", ""),
			Password: value("DB_PASSWORD", ""),
			SSLMode:  value("DB_SSLMODE", "disable"),
		},
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{"HTTP_HOST", cfg.HTTP.Host},
		{"DB_HOST", cfg.DB.Host},
		{"DB_NAME", cfg.DB.Name},
		{"DB_USER", cfg.DB.User},
		{"DB_PASSWORD", cfg.DB.Password},
	} {
		if strings.TrimSpace(field.value) == "" {
			return Config{}, fmt.Errorf("%s must not be empty", field.name)
		}
	}

	var err error
	cfg.HTTP.Port, err = port("HTTP_PORT", "8081")
	if err != nil {
		return Config{}, err
	}
	cfg.DB.Port, err = port("DB_PORT", "6544")
	if err != nil {
		return Config{}, err
	}

	switch cfg.DB.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return Config{}, fmt.Errorf("DB_SSLMODE must be disable, allow, prefer, require, verify-ca or verify-full")
	}

	return cfg, nil
}

func value(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return fallback
}

func port(name, fallback string) (int, error) {
	v := value(name, fallback)
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		// strconv errors include the input, which may accidentally contain a secret.
		return 0, fmt.Errorf("%s must be an integer between 1 and 65535", name)
	}
	return n, nil
}
