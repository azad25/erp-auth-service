package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"erp-auth-service/internal/models"
)

// Migration represents a database migration
type Migration struct {
	ID          uint      `gorm:"primaryKey"`
	Version     string    `gorm:"unique;not null;size:50"`
	Description string    `gorm:"not null;size:255"`
	AppliedAt   time.Time `gorm:"not null"`
	Checksum    string    `gorm:"not null;size:64"` // SHA256 of migration content
}

// MigrationManager handles database migrations
type MigrationManager struct {
	db *gorm.DB
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *gorm.DB) *MigrationManager {
	return &MigrationManager{db: db}
}

// InitializeMigrationTable creates the migrations table if it doesn't exist
func (m *MigrationManager) InitializeMigrationTable() error {
	return m.db.AutoMigrate(&Migration{})
}

// RunMigrations executes all pending migrations
func (m *MigrationManager) RunMigrations() error {
	// Initialize migration table
	if err := m.InitializeMigrationTable(); err != nil {
		return fmt.Errorf("failed to initialize migration table: %w", err)
	}

	migrations := []MigrationFunc{
		{
			Version:     "001_initial_schema",
			Description: "Create initial database schema with optimized indexes",
			Up:          m.migration001InitialSchema,
		},
		{
			Version:     "002_add_token_table",
			Description: "Add token table for JWT management",
			Up:          m.migration002AddTokenTable,
		},
		{
			Version:     "003_add_performance_indexes",
			Description: "Add performance-optimized indexes",
			Up:          m.migration003AddPerformanceIndexes,
		},
		{
			Version:     "004_create_materialized_views",
			Description: "Create materialized views for complex queries",
			Up:          m.migration004CreateMaterializedViews,
		},
	}

	for _, migration := range migrations {
		if err := m.runMigration(migration); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// MigrationFunc represents a migration function
type MigrationFunc struct {
	Version     string
	Description string
	Up          func() error
}

// runMigration executes a single migration if it hasn't been applied
func (m *MigrationManager) runMigration(migration MigrationFunc) error {
	// Check if migration has already been applied
	var existingMigration Migration
	err := m.db.Where("version = ?", migration.Version).First(&existingMigration).Error
	if err == nil {
		log.Printf("Migration %s already applied, skipping", migration.Version)
		return nil
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	// Run the migration
	log.Printf("Running migration: %s - %s", migration.Version, migration.Description)
	if err := migration.Up(); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Record the migration
	migrationRecord := Migration{
		Version:     migration.Version,
		Description: migration.Description,
		AppliedAt:   time.Now(),
		Checksum:    "placeholder", // In production, calculate actual checksum
	}

	if err := m.db.Create(&migrationRecord).Error; err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	log.Printf("Migration %s completed successfully", migration.Version)
	return nil
}

// migration001InitialSchema creates the initial database schema
func (m *MigrationManager) migration001InitialSchema() error {
	// Create all tables with optimized structure
	return m.db.AutoMigrate(
		&models.Organization{},
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.UserRole{},
		&models.RolePermission{},
	)
}

// migration002AddTokenTable creates the token table
func (m *MigrationManager) migration002AddTokenTable() error {
	return m.db.AutoMigrate(&models.Token{})
}

// migration003AddPerformanceIndexes adds performance-optimized indexes
func (m *MigrationManager) migration003AddPerformanceIndexes() error {
	// Composite indexes for common query patterns
	indexes := []string{
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email_active ON users(email) WHERE is_active = true",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_org_active ON users(organization_id, is_active)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id) INCLUDE (role_id)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_permissions_resource_action ON permissions(resource, action)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tokens_hash_active ON tokens(token_hash) WHERE revoked_at IS NULL",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tokens_user_type_active ON tokens(user_id, token_type) WHERE revoked_at IS NULL AND expires_at > NOW()",
		
		// Partial indexes for performance
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_last_login_recent ON users(last_login_at) WHERE last_login_at > NOW() - INTERVAL '30 days'",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_roles_org_active ON roles(organization_id) WHERE is_active = true",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_permissions_system ON permissions(is_system, resource, action)",
	}

	for _, indexSQL := range indexes {
		if err := m.db.Exec(indexSQL).Error; err != nil {
			log.Printf("Warning: Failed to create index: %s, error: %v", indexSQL, err)
			// Continue with other indexes even if one fails
		}
	}

	return nil
}

// migration004CreateMaterializedViews creates materialized views for complex queries
func (m *MigrationManager) migration004CreateMaterializedViews() error {
	// Create materialized view for user permissions (frequently queried)
	userPermissionsView := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS user_permissions_mv AS
		SELECT 
			ur.user_id,
			p.id as permission_id,
			p.name as permission_name,
			p.resource,
			p.action,
			p.scope,
			r.organization_id
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		JOIN role_permissions rp ON r.id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE r.is_active = true AND p.is_system = false;
	`

	if err := m.db.Exec(userPermissionsView).Error; err != nil {
		return fmt.Errorf("failed to create user_permissions_mv: %w", err)
	}

	// Create unique index on the materialized view
	userPermissionsIndex := `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_permissions_mv_unique 
		ON user_permissions_mv(user_id, permission_id);
	`

	if err := m.db.Exec(userPermissionsIndex).Error; err != nil {
		return fmt.Errorf("failed to create index on user_permissions_mv: %w", err)
	}

	// Create materialized view for organization statistics
	orgStatsView := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS organization_stats_mv AS
		SELECT 
			o.id as organization_id,
			o.name as organization_name,
			o.domain,
			COUNT(DISTINCT u.id) as user_count,
			COUNT(DISTINCT r.id) as role_count,
			COUNT(DISTINCT CASE WHEN u.last_login_at > NOW() - INTERVAL '30 days' THEN u.id END) as active_users_30d,
			MAX(u.last_login_at) as last_user_login
		FROM organizations o
		LEFT JOIN users u ON o.id = u.organization_id AND u.is_active = true
		LEFT JOIN roles r ON o.id = r.organization_id AND r.is_active = true
		WHERE o.is_active = true
		GROUP BY o.id, o.name, o.domain;
	`

	if err := m.db.Exec(orgStatsView).Error; err != nil {
		return fmt.Errorf("failed to create organization_stats_mv: %w", err)
	}

	// Create index on organization stats view
	orgStatsIndex := `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_stats_mv_org_id 
		ON organization_stats_mv(organization_id);
	`

	if err := m.db.Exec(orgStatsIndex).Error; err != nil {
		return fmt.Errorf("failed to create index on organization_stats_mv: %w", err)
	}

	return nil
}

// RefreshMaterializedViews refreshes all materialized views
func (m *MigrationManager) RefreshMaterializedViews() error {
	views := []string{
		"REFRESH MATERIALIZED VIEW CONCURRENTLY user_permissions_mv",
		"REFRESH MATERIALIZED VIEW CONCURRENTLY organization_stats_mv",
	}

	for _, viewSQL := range views {
		if err := m.db.Exec(viewSQL).Error; err != nil {
			log.Printf("Warning: Failed to refresh materialized view: %s, error: %v", viewSQL, err)
		}
	}

	return nil
}

// RollbackMigration rolls back a specific migration (for development/testing)
func (m *MigrationManager) RollbackMigration(version string) error {
	// This is a simplified rollback - in production, you'd want more sophisticated rollback logic
	var migration Migration
	err := m.db.Where("version = ?", version).First(&migration).Error
	if err != nil {
		return fmt.Errorf("migration %s not found: %w", version, err)
	}

	// Delete the migration record
	if err := m.db.Delete(&migration).Error; err != nil {
		return fmt.Errorf("failed to delete migration record: %w", err)
	}

	log.Printf("Migration %s rolled back", version)
	return nil
}

// GetAppliedMigrations returns all applied migrations
func (m *MigrationManager) GetAppliedMigrations() ([]Migration, error) {
	var migrations []Migration
	err := m.db.Order("applied_at ASC").Find(&migrations).Error
	return migrations, err
}