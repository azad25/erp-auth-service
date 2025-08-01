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
	Secret              string
	AccessExpiry        int
	RefreshExpiry       int
	KeyRotationEnabled  bool
	KeyRotationInterval int // hours
	RevokeRefreshOnUse  bool
}

type ServerConfig struct {
	Port           string
	GinMode        string
	AllowedOrigins []string
}

type GRPCConfig struct {
	Port                    string
	MaxConnectionIdle       int // seconds
	MaxConnectionAge        int // seconds
	MaxConnectionAgeGrace   int // seconds
	Time                    int // seconds
	Timeout                 int // seconds
	MaxRecvMsgSize          int // bytes
	MaxSendMsgSize          int // bytes
	MaxConcurrentStreams    int
	ConnectionTimeout       int // seconds
	KeepaliveEnforcementMinTime int // seconds
	KeepaliveEnforcementPermitWithoutStream bool
}

type KafkaConfig struct {
	Brokers                []string
	Topic                  string
	RetryAttempts          int
	RetryBackoffMs         int
	BatchSize              int
	BatchTimeoutMs         int
	ConnectionPoolSize     int
	LocalQueueSize         int
	DeadLetterTopic        string
	EnableLocalPersistence bool
	LocalStoragePath       string
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
			Secret:              getEnv("JWT_SECRET", "your-super-secret-jwt-key"),
			AccessExpiry:        getEnvAsInt("JWT_ACCESS_EXPIRY", 3600),
			RefreshExpiry:       getEnvAsInt("JWT_REFRESH_EXPIRY", 604800),
			KeyRotationEnabled:  getEnvAsBool("JWT_KEY_ROTATION_ENABLED", false),
			KeyRotationInterval: getEnvAsInt("JWT_KEY_ROTATION_INTERVAL", 24),
			RevokeRefreshOnUse:  getEnvAsBool("JWT_REVOKE_REFRESH_ON_USE", false),
		},
		Server: ServerConfig{
			Port:           getEnv("PORT", "8080"),
			GinMode:        getEnv("GIN_MODE", "debug"),
			AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "http://localhost:3000"), ","),
		},
		GRPC: GRPCConfig{
			Port:                    getEnv("GRPC_PORT", "9090"),
			MaxConnectionIdle:       getEnvAsInt("GRPC_MAX_CONNECTION_IDLE", 300),
			MaxConnectionAge:        getEnvAsInt("GRPC_MAX_CONNECTION_AGE", 300),
			MaxConnectionAgeGrace:   getEnvAsInt("GRPC_MAX_CONNECTION_AGE_GRACE", 5),
			Time:                    getEnvAsInt("GRPC_KEEPALIVE_TIME", 30),
			Timeout:                 getEnvAsInt("GRPC_KEEPALIVE_TIMEOUT", 5),
			MaxRecvMsgSize:          getEnvAsInt("GRPC_MAX_RECV_MSG_SIZE", 4194304), // 4MB
			MaxSendMsgSize:          getEnvAsInt("GRPC_MAX_SEND_MSG_SIZE", 4194304), // 4MB
			MaxConcurrentStreams:    getEnvAsInt("GRPC_MAX_CONCURRENT_STREAMS", 1000),
			ConnectionTimeout:       getEnvAsInt("GRPC_CONNECTION_TIMEOUT", 5),
			KeepaliveEnforcementMinTime: getEnvAsInt("GRPC_KEEPALIVE_ENFORCEMENT_MIN_TIME", 5),
			KeepaliveEnforcementPermitWithoutStream: getEnvAsBool("GRPC_KEEPALIVE_ENFORCEMENT_PERMIT_WITHOUT_STREAM", false),
		},
		Kafka: KafkaConfig{
			Brokers:                strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
			Topic:                  getEnv("KAFKA_TOPIC", "auth-events"),
			RetryAttempts:          getEnvAsInt("KAFKA_RETRY_ATTEMPTS", 3),
			RetryBackoffMs:         getEnvAsInt("KAFKA_RETRY_BACKOFF_MS", 1000),
			BatchSize:              getEnvAsInt("KAFKA_BATCH_SIZE", 100),
			BatchTimeoutMs:         getEnvAsInt("KAFKA_BATCH_TIMEOUT_MS", 10),
			ConnectionPoolSize:     getEnvAsInt("KAFKA_CONNECTION_POOL_SIZE", 10),
			LocalQueueSize:         getEnvAsInt("KAFKA_LOCAL_QUEUE_SIZE", 10000),
			DeadLetterTopic:        getEnv("KAFKA_DEAD_LETTER_TOPIC", "auth-events-dlq"),
			EnableLocalPersistence: getEnvAsBool("KAFKA_ENABLE_LOCAL_PERSISTENCE", true),
			LocalStoragePath:       getEnv("KAFKA_LOCAL_STORAGE_PATH", "/tmp/kafka-events"),
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

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}