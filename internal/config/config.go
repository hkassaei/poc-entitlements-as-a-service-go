package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration.
type Config struct {
	Port                    int
	Host                    string
	DatabaseURL             string
	DBPoolSize              int
	RedisURL                string
	HSSURL                  string
	AuthTokenTTLSeconds     int
	FastAuthTokenTTLSeconds int
	TempTokenTTLSeconds     int
	KMSKeyRing              string
	KMSKeyName              string
	KMSLocation             string
	LocalKEKHex             string
	SupportedVersions       []string
	DefaultConfigValidity   int
	OperatorMCC             string
	OperatorMNC             string
	OperatorName            string
	GCPProjectID            string
	Environment             string
}

// LoadConfig loads configuration from environment variables with sensible defaults.
func LoadConfig() Config {
	return Config{
		Port:                    envInt("PORT", 8080),
		Host:                    envOrDefault("HOST", "0.0.0.0"),
		DatabaseURL:             envOrDefault("DATABASE_URL", "postgresql://ecs:password@localhost:5432/entitlements"),
		DBPoolSize:              envInt("DB_POOL_SIZE", 2),
		RedisURL:                envOrDefault("REDIS_URL", "redis://localhost:6379"),
		HSSURL:                  envOrDefault("HSS_URL", "http://localhost:3001"),
		AuthTokenTTLSeconds:     envInt("AUTH_TOKEN_TTL_SECONDS", 86400),
		FastAuthTokenTTLSeconds: envInt("FAST_AUTH_TOKEN_TTL_SECONDS", 172800),
		TempTokenTTLSeconds:     envInt("TEMP_TOKEN_TTL_SECONDS", 3600),
		KMSKeyRing:              envOrDefault("KMS_KEY_RING", "entitlement-keys"),
		KMSKeyName:              envOrDefault("KMS_KEY_NAME", "ki-kek"),
		KMSLocation:             envOrDefault("KMS_LOCATION", "us-central1"),
		LocalKEKHex:             envOrDefault("LOCAL_KEK_HEX", ""),
		SupportedVersions:       strings.Split(envOrDefault("SUPPORTED_VERSIONS", "2,4"), ","),
		DefaultConfigValidity:   envInt("DEFAULT_CONFIG_VALIDITY", 172800),
		OperatorMCC:             envOrDefault("OPERATOR_MCC", "001"),
		OperatorMNC:             envOrDefault("OPERATOR_MNC", "01"),
		OperatorName:            envOrDefault("OPERATOR_NAME", "TestOperator"),
		GCPProjectID:            envOrDefault("GCP_PROJECT_ID", ""),
		Environment:             envOrDefault("ENVIRONMENT", "development"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
