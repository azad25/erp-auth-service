package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Server   ServerConfig
	GRPC     GRPCConfig
	Kafka    KafkaConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret        string
	AccessExpiry  int
	RefreshExpiry int
}

type ServerConfig struct {
	Port           string
	GinMode        string
	AllowedOrigins []string
}

type GRPCConfig struct {
	Port string
}

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

func Load() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Name:     getEnv("DB_NAME", "erp_auth"),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:        getEnv("JWT_SECRET", "your-super-secret-jwt-key"),
			AccessExpiry:  getEnvAsInt("JWT_ACCESS_EXPIRY", 3600),
			RefreshExpiry: getEnvAsInt("JWT_REFRESH_EXPIRY", 604800),
		},
		Server: ServerConfig{
			Port:           getEnv("PORT", "8080"),
			GinMode:        getEnv("GIN_MODE", "debug"),
			AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "http://localhost:3000"), ","),
		},
		GRPC: GRPCConfig{
			Port: getEnv("GRPC_PORT", "9090"),
		},
		Kafka: KafkaConfig{
			Brokers: strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
			Topic:   getEnv("KAFKA_TOPIC", "auth-events"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}