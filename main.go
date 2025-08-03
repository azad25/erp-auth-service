package main

import (
	"flag"
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
	"erp-auth-service/internal/seeder"

	"github.com/joho/godotenv"
)

func main() {
	// Parse command line flags
	seedFlag := flag.Bool("seed", false, "Run database seeding")
	seedMinimal := flag.Bool("seed-minimal", false, "Run minimal database seeding")
	flag.Parse()

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

	// Handle seeding if requested via command line flags
	if *seedFlag || *seedMinimal {
		s := seeder.NewSeeder(db.GetWriteDB())
		
		if *seedMinimal {
			log.Println("🌱 Running minimal database seeding...")
			if err := s.SeedMinimal(); err != nil {
				log.Fatal("Failed to run minimal seeding:", err)
			}
		} else {
			log.Println("🌱 Running full database seeding...")
			if err := s.SeedAll(); err != nil {
				log.Fatal("Failed to run full seeding:", err)
			}
		}
		
		log.Println("✅ Seeding completed, exiting...")
		return
	}

	// Seed database with initial data on startup (if not already seeded)
	seederInstance := seeder.NewSeeder(db.GetWriteDB())
	if err := seederInstance.SeedAll(); err != nil {
		log.Printf("Warning: Failed to seed database: %v", err)
		// Don't fail startup if seeding fails, just log the warning
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

	// Context for graceful shutdown (simplified for now)
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	// Wait group for goroutines
	var wg sync.WaitGroup

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		// Initialize router with Kafka producer
		r := router.Initialize(db.GetWriteDB(), redisClient, cfg, kafkaProducer)

		log.Printf("Starting HTTP server on port %s", cfg.Server.Port)
		if err := r.Run(":" + cfg.Server.Port); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start gRPC server
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		grpcSrv := grpcServer.NewAuthGRPCServer(db.GetWriteDB(), redisClient, cfg)
		log.Printf("Starting gRPC server on port %s", cfg.GRPC.Port)
		if err := grpcSrv.Start(); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal received
	<-sigChan
	log.Println("Shutting down servers...")

	// Wait for all goroutines to finish
	wg.Wait()
	log.Println("Servers shut down gracefully")
}