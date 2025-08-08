package database

import (
	"database/sql"
	"fmt"
	"time"

	"erp-auth-service/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseManager manages database connections with connection pooling and read replicas
type DatabaseManager struct {
	WriteDB *gorm.DB
	ReadDB  *gorm.DB
}

// Initialize creates database connections with optimized connection pooling
func Initialize(cfg config.DatabaseConfig) (*DatabaseManager, error) {
	// Initialize write database (primary)
	writeDB, err := initializeConnection(cfg, "write")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize write database: %w", err)
	}

	// Initialize read database (replica) - for now, use same connection
	// In production, this would connect to a read replica
	readDB, err := initializeConnection(cfg, "read")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize read database: %w", err)
	}

	// Configure connection pooling for write database
	if err := configureConnectionPool(writeDB, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure write database pool: %w", err)
	}

	// Configure connection pooling for read database
	if err := configureConnectionPool(readDB, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure read database pool: %w", err)
	}

	dbManager := &DatabaseManager{
		WriteDB: writeDB,
		ReadDB:  readDB,
	}

	// Run migrations
	migrationManager := NewMigrationManager(writeDB)
	if err := migrationManager.RunMigrations(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return dbManager, nil
}

// initializeConnection creates a single database connection
func initializeConnection(cfg config.DatabaseConfig, connectionType string) (*gorm.DB, error) {
	// Build DSN - in production, read replicas would have different host/port
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)

	// Configure GORM with performance optimizations
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Use structured logging instead
		
		// Performance optimizations
		PrepareStmt:              true,  // Prepare statements for better performance
		DisableForeignKeyConstraintWhenMigrating: false, // Keep FK constraints for data integrity
		
		// Connection pool will be configured separately
		ConnPool: nil,
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s database: %w", connectionType, err)
	}

	return db, nil
}

// configureConnectionPool sets up optimized connection pooling
func configureConnectionPool(db *gorm.DB, cfg config.DatabaseConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Connection pool configuration for high-performance scenarios
	sqlDB.SetMaxOpenConns(50)                  // Reduced from 100 to 50 for better stability
	sqlDB.SetMaxIdleConns(10)                  // Reduced from 25 to 10
	sqlDB.SetConnMaxLifetime(10 * time.Minute) // Increased from 5 to 10 minutes
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)  // Increased from 1 to 5 minutes

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

// GetWriteDB returns the write database connection
func (dm *DatabaseManager) GetWriteDB() *gorm.DB {
	return dm.WriteDB
}

// GetReadDB returns the read database connection
func (dm *DatabaseManager) GetReadDB() *gorm.DB {
	return dm.ReadDB
}

// Close closes all database connections
func (dm *DatabaseManager) Close() error {
	var errs []error

	if dm.WriteDB != nil {
		if sqlDB, err := dm.WriteDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close write database: %w", err))
			}
		}
	}

	if dm.ReadDB != nil {
		if sqlDB, err := dm.ReadDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close read database: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing database connections: %v", errs)
	}

	return nil
}

// HealthCheck performs a health check on both database connections
func (dm *DatabaseManager) HealthCheck() error {
	// Check write database
	if writeSQL, err := dm.WriteDB.DB(); err != nil {
		return fmt.Errorf("failed to get write database connection: %w", err)
	} else if err := writeSQL.Ping(); err != nil {
		return fmt.Errorf("write database health check failed: %w", err)
	}

	// Check read database
	if readSQL, err := dm.ReadDB.DB(); err != nil {
		return fmt.Errorf("failed to get read database connection: %w", err)
	} else if err := readSQL.Ping(); err != nil {
		return fmt.Errorf("read database health check failed: %w", err)
	}

	return nil
}

// GetConnectionStats returns connection pool statistics
func (dm *DatabaseManager) GetConnectionStats() (map[string]sql.DBStats, error) {
	stats := make(map[string]sql.DBStats)

	if writeSQL, err := dm.WriteDB.DB(); err == nil {
		stats["write"] = writeSQL.Stats()
	} else {
		return nil, fmt.Errorf("failed to get write database stats: %w", err)
	}

	if readSQL, err := dm.ReadDB.DB(); err == nil {
		stats["read"] = readSQL.Stats()
	} else {
		return nil, fmt.Errorf("failed to get read database stats: %w", err)
	}

	return stats, nil
}