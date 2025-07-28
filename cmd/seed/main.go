package main

import (
	"flag"
	"log"
	"os"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/database"
	"erp-auth-service/internal/seeder"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// Parse command line flags
	minimal := flag.Bool("minimal", false, "Run minimal seeding (for testing)")
	force := flag.Bool("force", false, "Force seeding even if data exists")
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

	// Get database instance for connection management
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Failed to get database instance:", err)
	}
	defer sqlDB.Close()

	// Create seeder
	s := seeder.NewSeeder(db)

	// Check if we should force seeding
	if *force {
		log.Println("🔄 Force flag detected, clearing existing data...")
		if err := clearDatabase(db); err != nil {
			log.Fatal("Failed to clear database:", err)
		}
	}

	// Run seeding
	if *minimal {
		log.Println("🌱 Running minimal seeding...")
		if err := s.SeedMinimal(); err != nil {
			log.Fatal("Failed to run minimal seeding:", err)
		}
	} else {
		log.Println("🌱 Running full seeding...")
		if err := s.SeedAll(); err != nil {
			log.Fatal("Failed to run full seeding:", err)
		}
	}

	log.Println("🎉 Seeding completed successfully!")
}

func clearDatabase(db *gorm.DB) error {
	log.Println("⚠️  Clearing existing data...")

	// Drop tables in reverse order of dependencies
	tables := []string{
		"role_permissions",
		"user_roles", 
		"permissions",
		"roles",
		"users",
		"organizations",
	}

	for _, table := range tables {
		if err := db.Exec("DROP TABLE IF EXISTS " + table + " CASCADE").Error; err != nil {
			return err
		}
		log.Printf("Dropped table: %s", table)
	}

	// Re-run migrations
	log.Println("🔄 Re-running migrations...")
	if err := database.Initialize(config.Load().Database); err != nil {
		return err
	}

	log.Println("✅ Database cleared and re-initialized")
	return nil
}