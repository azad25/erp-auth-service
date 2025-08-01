package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/events"
	"erp-auth-service/internal/models"
)

// MockUserRepository is a mock implementation of UserRepository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetByEmailWithOrganization(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) Create(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) Update(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockUserRepository) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Permission), args.Error(1)
}

func (m *MockUserRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Role), args.Error(1)
}

func (m *MockUserRepository) GetActiveUsersByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]models.User, error) {
	args := m.Called(ctx, orgID, limit, offset)
	return args.Get(0).([]models.User), args.Error(1)
}

func (m *MockUserRepository) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]models.User, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).([]models.User), args.Error(1)
}

func (m *MockUserRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID, loginTime time.Time) error {
	args := m.Called(ctx, userID, loginTime)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockUserRepository) ResetFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockUserRepository) LockUser(ctx context.Context, userID uuid.UUID, lockUntil time.Time) error {
	args := m.Called(ctx, userID, lockUntil)
	return args.Error(0)
}

// MockOrganizationRepository is a mock implementation of OrganizationRepository
type MockOrganizationRepository struct {
	mock.Mock
}

func (m *MockOrganizationRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Organization), args.Error(1)
}

func (m *MockOrganizationRepository) GetByDomain(ctx context.Context, domain string) (*models.Organization, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Organization), args.Error(1)
}

func (m *MockOrganizationRepository) Create(ctx context.Context, org *models.Organization) error {
	args := m.Called(ctx, org)
	return args.Error(0)
}

func (m *MockOrganizationRepository) Update(ctx context.Context, org *models.Organization) error {
	args := m.Called(ctx, org)
	return args.Error(0)
}

func (m *MockOrganizationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockOrganizationRepository) GetActiveOrganizations(ctx context.Context, limit, offset int) ([]models.Organization, error) {
	args := m.Called(ctx, limit, offset)
	return args.Get(0).([]models.Organization), args.Error(1)
}

func (m *MockOrganizationRepository) GetOrganizationWithUsers(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Organization), args.Error(1)
}

func (m *MockOrganizationRepository) GetUserCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	args := m.Called(ctx, orgID)
	return args.Int(0), args.Error(1)
}

// MockRoleRepository is a mock implementation of RoleRepository
type MockRoleRepository struct {
	mock.Mock
}

func (m *MockRoleRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Role, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Role), args.Error(1)
}

func (m *MockRoleRepository) GetByName(ctx context.Context, name string, orgID uuid.UUID) (*models.Role, error) {
	args := m.Called(ctx, name, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Role), args.Error(1)
}

func (m *MockRoleRepository) Create(ctx context.Context, role *models.Role) error {
	args := m.Called(ctx, role)
	return args.Error(0)
}

func (m *MockRoleRepository) Update(ctx context.Context, role *models.Role) error {
	args := m.Called(ctx, role)
	return args.Error(0)
}

func (m *MockRoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRoleRepository) GetRolesByOrganization(ctx context.Context, orgID uuid.UUID) ([]models.Role, error) {
	args := m.Called(ctx, orgID)
	return args.Get(0).([]models.Role), args.Error(1)
}

func (m *MockRoleRepository) GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*models.Role, error) {
	args := m.Called(ctx, roleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Role), args.Error(1)
}

func (m *MockRoleRepository) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error) {
	args := m.Called(ctx, roleID)
	return args.Get(0).([]models.Permission), args.Error(1)
}

func (m *MockRoleRepository) AssignPermissionToRole(ctx context.Context, roleID, permissionID uuid.UUID) error {
	args := m.Called(ctx, roleID, permissionID)
	return args.Error(0)
}

func (m *MockRoleRepository) RevokePermissionFromRole(ctx context.Context, roleID, permissionID uuid.UUID) error {
	args := m.Called(ctx, roleID, permissionID)
	return args.Error(0)
}

func (m *MockRoleRepository) AssignRoleToUser(ctx context.Context, userID, roleID uuid.UUID) error {
	args := m.Called(ctx, userID, roleID)
	return args.Error(0)
}

func (m *MockRoleRepository) RevokeRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error {
	args := m.Called(ctx, userID, roleID)
	return args.Error(0)
}

// MockTokenService is a mock implementation of TokenService
type MockTokenService struct {
	mock.Mock
}

func (m *MockTokenService) GenerateTokenPair(ctx context.Context, userID, organizationID uuid.UUID, email string, securityCtx *SecurityContext) (*entities.TokenPair, error) {
	args := m.Called(ctx, userID, organizationID, email, securityCtx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entities.TokenPair), args.Error(1)
}

func (m *MockTokenService) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType entities.TokenType) error {
	args := m.Called(ctx, userID, tokenType)
	return args.Error(0)
}

// MockEventPublisher is a mock implementation of EventPublisher
type MockEventPublisher struct {
	mock.Mock
}

func (m *MockEventPublisher) PublishUserLoggedIn(ctx context.Context, data events.UserLoggedInData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	args := m.Called(ctx, data, userID, orgID, ipAddress, userAgent)
	return args.Error(0)
}

func (m *MockEventPublisher) PublishUserRegistered(ctx context.Context, data events.UserRegisteredData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	args := m.Called(ctx, data, userID, orgID, ipAddress, userAgent)
	return args.Error(0)
}

func (m *MockEventPublisher) PublishOrganizationCreated(ctx context.Context, data events.OrganizationCreatedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	args := m.Called(ctx, data, userID, orgID, ipAddress, userAgent)
	return args.Error(0)
}

func (m *MockEventPublisher) PublishPasswordChanged(ctx context.Context, data events.PasswordChangedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	args := m.Called(ctx, data, userID, orgID, ipAddress, userAgent)
	return args.Error(0)
}

// MockCacheManager is a mock implementation of CacheManager
type MockCacheManager struct {
	mock.Mock
	data map[string][]byte
}

func NewMockCacheManager() *MockCacheManager {
	return &MockCacheManager{
		data: make(map[string][]byte),
	}
}

func (m *MockCacheManager) Get(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	if data, exists := m.data[key]; exists {
		return data, nil
	}
	return nil, args.Error(1)
}

func (m *MockCacheManager) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	args := m.Called(ctx, key, value, ttl)
	// Don't actually store data to avoid race conditions in tests
	return args.Error(0)
}

func (m *MockCacheManager) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	delete(m.data, key)
	return args.Error(0)
}

func (m *MockCacheManager) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	_, exists := m.data[key]
	return exists, args.Error(1)
}

func (m *MockCacheManager) Clear(ctx context.Context) error {
	args := m.Called(ctx)
	m.data = make(map[string][]byte)
	return args.Error(0)
}

func (m *MockCacheManager) Stats() cache.CacheStats {
	args := m.Called()
	return args.Get(0).(cache.CacheStats)
}

func (m *MockCacheManager) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockCacheManager) GetFromLevel(ctx context.Context, key string, level cache.CacheLevel) ([]byte, error) {
	args := m.Called(ctx, key, level)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCacheManager) SetToLevel(ctx context.Context, key string, value []byte, ttl time.Duration, level cache.CacheLevel) error {
	args := m.Called(ctx, key, value, ttl, level)
	return args.Error(0)
}

func (m *MockCacheManager) InvalidateKey(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockCacheManager) InvalidatePattern(ctx context.Context, pattern string) error {
	args := m.Called(ctx, pattern)
	return args.Error(0)
}

func (m *MockCacheManager) WarmCache(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

// MockTokenRepository is a mock implementation of TokenRepository
type MockTokenRepository struct {
	mock.Mock
}

func (m *MockTokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Token, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Token), args.Error(1)
}

func (m *MockTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.Token, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Token), args.Error(1)
}

func (m *MockTokenRepository) Create(ctx context.Context, token *models.Token) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockTokenRepository) Update(ctx context.Context, token *models.Token) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockTokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockTokenRepository) RevokeToken(ctx context.Context, tokenHash string) error {
	args := m.Called(ctx, tokenHash)
	return args.Error(0)
}

func (m *MockTokenRepository) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType models.TokenType) error {
	args := m.Called(ctx, userID, tokenType)
	return args.Error(0)
}

func (m *MockTokenRepository) GetActiveTokensByUser(ctx context.Context, userID uuid.UUID, tokenType models.TokenType) ([]models.Token, error) {
	args := m.Called(ctx, userID, tokenType)
	return args.Get(0).([]models.Token), args.Error(1)
}

func (m *MockTokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTokenRepository) DeleteRevokedTokens(ctx context.Context, olderThan time.Time) error {
	args := m.Called(ctx, olderThan)
	return args.Error(0)
}

// MockPermissionRepository is a mock implementation of PermissionRepository
type MockPermissionRepository struct {
	mock.Mock
}

func (m *MockPermissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Permission, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetByName(ctx context.Context, name string) (*models.Permission, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetByResourceAndAction(ctx context.Context, resource, action string) (*models.Permission, error) {
	args := m.Called(ctx, resource, action)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) Create(ctx context.Context, permission *models.Permission) error {
	args := m.Called(ctx, permission)
	return args.Error(0)
}

func (m *MockPermissionRepository) Update(ctx context.Context, permission *models.Permission) error {
	args := m.Called(ctx, permission)
	return args.Error(0)
}

func (m *MockPermissionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockPermissionRepository) GetPermissionsByResource(ctx context.Context, resource string) ([]models.Permission, error) {
	args := m.Called(ctx, resource)
	return args.Get(0).([]models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetPermissionsByRole(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error) {
	args := m.Called(ctx, roleID)
	return args.Get(0).([]models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetChildPermissions(ctx context.Context, parentID uuid.UUID) ([]models.Permission, error) {
	args := m.Called(ctx, parentID)
	return args.Get(0).([]models.Permission), args.Error(1)
}

func (m *MockPermissionRepository) GetPermissionHierarchy(ctx context.Context, permissionID uuid.UUID) ([]models.Permission, error) {
	args := m.Called(ctx, permissionID)
	return args.Get(0).([]models.Permission), args.Error(1)
}