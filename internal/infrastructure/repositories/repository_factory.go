package repositories

import (
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
)

// RepositoryFactory manages all repository instances
type RepositoryFactory struct {
	writeDB *gorm.DB
	readDB  *gorm.DB
	
	// Repository instances
	userRepo           interfaces.UserRepository
	organizationRepo   interfaces.OrganizationRepository
	roleRepo           interfaces.RoleRepository
	permissionRepo     interfaces.PermissionRepository
	tokenRepo          interfaces.TokenRepository
	userRoleRepo       interfaces.UserRoleRepository
	rolePermissionRepo interfaces.RolePermissionRepository
}

// NewRepositoryFactory creates a new repository factory
func NewRepositoryFactory(writeDB, readDB *gorm.DB) *RepositoryFactory {
	return &RepositoryFactory{
		writeDB: writeDB,
		readDB:  readDB,
	}
}

// UserRepository returns the user repository instance
func (f *RepositoryFactory) UserRepository() interfaces.UserRepository {
	if f.userRepo == nil {
		f.userRepo = NewUserRepository(f.writeDB, f.readDB)
	}
	return f.userRepo
}

// OrganizationRepository returns the organization repository instance
func (f *RepositoryFactory) OrganizationRepository() interfaces.OrganizationRepository {
	if f.organizationRepo == nil {
		f.organizationRepo = NewOrganizationRepository(f.writeDB, f.readDB)
	}
	return f.organizationRepo
}

// RoleRepository returns the role repository instance
func (f *RepositoryFactory) RoleRepository() interfaces.RoleRepository {
	if f.roleRepo == nil {
		f.roleRepo = NewRoleRepository(f.writeDB, f.readDB)
	}
	return f.roleRepo
}

// PermissionRepository returns the permission repository instance
func (f *RepositoryFactory) PermissionRepository() interfaces.PermissionRepository {
	if f.permissionRepo == nil {
		f.permissionRepo = NewPermissionRepository(f.writeDB, f.readDB)
	}
	return f.permissionRepo
}

// TokenRepository returns the token repository instance
func (f *RepositoryFactory) TokenRepository() interfaces.TokenRepository {
	if f.tokenRepo == nil {
		f.tokenRepo = NewTokenRepository(f.writeDB, f.readDB)
	}
	return f.tokenRepo
}

// UserRoleRepository returns the user role repository instance
func (f *RepositoryFactory) UserRoleRepository() interfaces.UserRoleRepository {
	if f.userRoleRepo == nil {
		f.userRoleRepo = NewUserRoleRepository(f.writeDB, f.readDB)
	}
	return f.userRoleRepo
}

// RolePermissionRepository returns the role permission repository instance
func (f *RepositoryFactory) RolePermissionRepository() interfaces.RolePermissionRepository {
	if f.rolePermissionRepo == nil {
		f.rolePermissionRepo = NewRolePermissionRepository(f.writeDB, f.readDB)
	}
	return f.rolePermissionRepo
}