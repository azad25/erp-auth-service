package main

import (
	"log"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/database"
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

	// Initialize database
	dbManager, err := database.Initialize(cfg.Database)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Get the GORM database instance
	db := dbManager.GetWriteDB()

	// Get database instance for connection management
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Failed to get database instance:", err)
	}
	defer sqlDB.Close()

	// Create seeder
	s := seeder.NewSeeder(db)

	log.Println("🌱 Running full seeding...")

	if err := s.SeedAll(); err != nil {
		log.Fatal("Failed to run full seeding:", err)
	}

	log.Println("🎉 Seeding completed successfully!")
}
