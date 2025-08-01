package interfaces

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

// Database defines the interface for database operations
type Database interface {
	GetDB() *gorm.DB
	Close() error
	Ping(ctx context.Context) error
	BeginTx(ctx context.Context) (Transaction, error)
	Migrate() error
	Health() error
}

// Transaction defines the interface for database transactions
type Transaction interface {
	Commit() error
	Rollback() error
	GetDB() *gorm.DB
}

// QueryBuilder defines the interface for building complex queries
type QueryBuilder interface {
	Select(fields ...string) QueryBuilder
	Where(condition string, args ...interface{}) QueryBuilder
	Join(table, condition string) QueryBuilder
	LeftJoin(table, condition string) QueryBuilder
	OrderBy(field string, desc bool) QueryBuilder
	Limit(limit int) QueryBuilder
	Offset(offset int) QueryBuilder
	GroupBy(fields ...string) QueryBuilder
	Having(condition string, args ...interface{}) QueryBuilder
	Build() (string, []interface{})
}

// ConnectionPool defines the interface for database connection pooling
type ConnectionPool interface {
	GetConnection(ctx context.Context) (*sql.DB, error)
	ReleaseConnection(conn *sql.DB) error
	Stats() ConnectionStats
	Close() error
}

// ConnectionStats provides statistics about the connection pool
type ConnectionStats struct {
	OpenConnections int
	InUse          int
	Idle           int
	WaitCount      int64
	WaitDuration   int64
	MaxIdleClosed  int64
	MaxLifetimeClosed int64
}