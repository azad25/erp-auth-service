package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/database"
	"erp-auth-service/internal/events"
	grpcServer "erp-auth-service/internal/grpc"
	"erp-auth-service/internal/logging"
	"erp-auth-service/internal/redis"
	"erp-auth-service/internal/router"
	"erp-auth-service/internal/seeder"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize configuration
	cfg := config.Load()

	// Initialize logging system
	loggingConfig := logging.DefaultLoggingServiceConfig()
	if err := logging.InitializeGlobalLogger(loggingConfig); err != nil {
		log.Printf("Warning: Failed to initialize logging service: %v", err)
	}
	defer logging.CloseGlobalLogger()

	logger := logging.GetGlobalLogger()
	ctx := context.Background()

	logger.Info(ctx, "Starting ERP Auth Service", map[string]interface{}{
		"version": "1.0.0",
		"port":    cfg.Server.Port,
	})

	// Initialize database
	db, err := database.Initialize(cfg.Database)
	if err != nil {
		logger.Fatal(ctx, "Failed to initialize database", map[string]interface{}{
			"error": err.Error(),
		})
	}

	logger.Info(ctx, "Database initialized successfully", nil)

	// Seed database with initial data on startup (if not already seeded)
	seederInstance := seeder.NewSeeder(db.GetWriteDB())
	if err := seederInstance.SeedAll(); err != nil {
		logger.Warn(ctx, "Failed to seed database", map[string]interface{}{
			"error": err.Error(),
		})
		// Don't fail startup if seeding fails, just log the warning
	}

	// Initialize Redis
	redisClient, err := redis.Initialize(cfg.Redis)
	if err != nil {
		logger.Fatal(ctx, "Failed to initialize Redis", map[string]interface{}{
			"error": err.Error(),
		})
	}

	logger.Info(ctx, "Redis initialized successfully", nil)

	// Initialize Kafka producer
	kafkaProducer := events.NewProducer(events.ProducerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topic,
	})
	defer kafkaProducer.Close()

	logger.Info(ctx, "Kafka producer initialized successfully", map[string]interface{}{
		"brokers": cfg.Kafka.Brokers,
		"topic":   cfg.Kafka.Topic,
	})

	// Wait group for goroutines
	var wg sync.WaitGroup

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Initialize router with Kafka producer
		r := router.Initialize(db.GetWriteDB(), redisClient, cfg, kafkaProducer)

		logger.Info(ctx, "Starting HTTP server", map[string]interface{}{
			"port": cfg.Server.Port,
		})

		if err := r.Run(":" + cfg.Server.Port); err != nil {
			logger.Error(ctx, "HTTP server error", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()

	// Start gRPC server
	wg.Add(1)
	go func() {
		defer wg.Done()

		grpcSrv := grpcServer.NewAuthGRPCServer(db.GetWriteDB(), redisClient, cfg)

		logger.Info(ctx, "Starting gRPC server", map[string]interface{}{
			"port": cfg.GRPC.Port,
		})

		if err := grpcSrv.Start(); err != nil {
			logger.Error(ctx, "gRPC server error", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal received
	<-sigChan
	logger.Info(ctx, "Received shutdown signal, shutting down servers", nil)

	// Wait for all goroutines to finish
	wg.Wait()
	logger.Info(ctx, "Servers shut down gracefully", nil)
}
