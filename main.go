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
	"erp-auth-service/internal/redis"
	"erp-auth-service/internal/router"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Initialize(cfg.Database)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Initialize Redis
	redisClient, err := redis.Initialize(cfg.Redis)
	if err != nil {
		log.Fatal("Failed to initialize Redis:", err)
	}

	// Initialize Kafka producer
	kafkaProducer := events.NewProducer(events.ProducerConfig{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topic,
	})
	defer kafkaProducer.Close()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wait group for goroutines
	var wg sync.WaitGroup

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		// Initialize router with Kafka producer
		r := router.Initialize(db, redisClient, cfg, kafkaProducer)

		log.Printf("Starting HTTP server on port %s", cfg.Server.Port)
		if err := r.Run(":" + cfg.Server.Port); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start gRPC server
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		grpcSrv := grpcServer.NewAuthGRPCServer(db, redisClient, cfg)
		log.Printf("Starting gRPC server on port %s", cfg.GRPC.Port)
		if err := grpcSrv.Start(cfg.GRPC.Port); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal received
	<-sigChan
	log.Println("Shutting down servers...")

	// Cancel context to signal shutdown
	cancel()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Println("Servers shut down gracefully")
}