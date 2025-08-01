package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"erp-auth-service/internal/models"
)

// UserRepositoryTestSuite defines the test suite for user repository
type UserRepositoryTestSuite struct {
	suite.Suite
	db         *gorm.DB
	readDB     *gorm.DB
	repository *userRepository
	ctx        context.Context
}

// SetupSuite sets up the test suite
func (suite *UserRepositoryTestSuite) SetupSuite() {
	// Use in-memory SQLite for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	suite.Require().NoError(err)

	// Use same DB for read operations in tests
	readDB := db

	// Auto-migrate test schema
	err = db.AutoMigrate(
		&models.Organization{},
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.UserRole{},
		&models.RolePermission{},
	)
	suite.Require().NoError(err)

	suite.db = db
	suite.readDB = readDB
	suite.repository = &userRepository{db: db, readDB: readDB}
	suite.ctx = context.Background()
}

// TearDownSuite cleans up after tests
func (suite *UserRepositoryTestSuite) TearDownSuite() {
	sqlDB, err := suite.db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// SetupTest sets up each individual test
func (suite *UserRepositoryTestSuite) SetupTest() {
	// Clean up tables before each test
	suite.db.Exec("DELETE FROM user_roles")
	suite.db.Exec("DELETE FROM role_permissions")
	suite.db.Exec("DELETE FROM users")
	suite.db.Exec("DELETE FROM roles")
	suite.db.Exec("DELETE FROM permissions")
	suite.db.Exec("DELETE FROM organizations")
}

// TestCreateUser tests user creation
func (suite *UserRepositoryTestSuite) TestCreateUser() {
	// Create test organization using SQL to avoid Settings struct issue
	orgID := uuid.New()
	err := suite.db.Exec("INSERT INTO organizations (id, name, domain, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		orgID.String(), "Test Org", "test.com", true, time.Now(), time.Now()).Error
	suite.Require().NoError(err)
	
	var org models.Organization
	if err := suite.db.Where("id = ?", orgID).First(&org).Error; err != nil {
		suite.Require().NoError(err)
	}

	// Create test user
	user := &models.User{
		ID:             uuid.New(),
		OrganizationID: org.ID,
		Email:          "test@test.com",
		PasswordHash:   "hashedpassword",
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
	}

	err = suite.repository.Create(suite.ctx, user)
	suite.Assert().NoError(err)
	suite.Assert().NotEqual(uuid.Nil, user.ID)

	// Verify user was created
	var createdUser models.User
	err = suite.db.Where("email = ?", user.Email).First(&createdUser).Error
	suite.Assert().NoError(err)
	suite.Assert().Equal(user.Email, createdUser.Email)
}

// TestGetByID tests retrieving user by ID
func (suite *UserRepositoryTestSuite) TestGetByID() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Test successful retrieval
	retrievedUser, err := suite.repository.GetByID(suite.ctx, user.ID)
	suite.Assert().NoError(err)
	suite.Assert().Equal(user.Email, retrievedUser.Email)
	suite.Assert().Equal(user.FirstName, retrievedUser.FirstName)

	// Test non-existent user
	nonExistentID := uuid.New()
	_, err = suite.repository.GetByID(suite.ctx, nonExistentID)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "user not found")
}

// TestGetByEmail tests retrieving user by email
func (suite *UserRepositoryTestSuite) TestGetByEmail() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Test successful retrieval
	retrievedUser, err := suite.repository.GetByEmail(suite.ctx, user.Email)
	suite.Assert().NoError(err)
	suite.Assert().Equal(user.ID, retrievedUser.ID)
	suite.Assert().Equal(user.FirstName, retrievedUser.FirstName)

	// Test non-existent email
	_, err = suite.repository.GetByEmail(suite.ctx, "nonexistent@test.com")
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "user not found")
}

// TestGetByEmailWithOrganization tests retrieving user with organization
func (suite *UserRepositoryTestSuite) TestGetByEmailWithOrganization() {
	// Create test data
	org, user := suite.createTestUserWithOrg()

	// Test successful retrieval with organization
	retrievedUser, err := suite.repository.GetByEmailWithOrganization(suite.ctx, user.Email)
	suite.Assert().NoError(err)
	suite.Assert().Equal(user.ID, retrievedUser.ID)
	suite.Assert().Equal(org.Name, retrievedUser.Organization.Name)
}

// TestUpdateUser tests user updates
func (suite *UserRepositoryTestSuite) TestUpdateUser() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Update user
	user.FirstName = "Updated"
	user.LastName = "Name"
	err := suite.repository.Update(suite.ctx, user)
	suite.Assert().NoError(err)

	// Verify update
	retrievedUser, err := suite.repository.GetByID(suite.ctx, user.ID)
	suite.Assert().NoError(err)
	suite.Assert().Equal("Updated", retrievedUser.FirstName)
	suite.Assert().Equal("Name", retrievedUser.LastName)
}

// TestDeleteUser tests user soft deletion
func (suite *UserRepositoryTestSuite) TestDeleteUser() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Delete user
	err := suite.repository.Delete(suite.ctx, user.ID)
	suite.Assert().NoError(err)

	// Verify user is soft deleted (not returned by GetByID)
	_, err = suite.repository.GetByID(suite.ctx, user.ID)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "user not found")

	// Verify user still exists in database but is inactive
	var dbUser models.User
	err = suite.db.Unscoped().Where("id = ?", user.ID).First(&dbUser).Error
	suite.Assert().NoError(err)
	suite.Assert().False(dbUser.IsActive)
}

// TestGetUsersByIDs tests bulk user retrieval
func (suite *UserRepositoryTestSuite) TestGetUsersByIDs() {
	// Create test data
	org := suite.createTestOrg()
	user1 := suite.createTestUser(org.ID, "user1@test.com")
	user2 := suite.createTestUser(org.ID, "user2@test.com")
	user3 := suite.createTestUser(org.ID, "user3@test.com")

	// Test bulk retrieval
	userIDs := []uuid.UUID{user1.ID, user2.ID, user3.ID}
	users, err := suite.repository.GetUsersByIDs(suite.ctx, userIDs)
	suite.Assert().NoError(err)
	suite.Assert().Len(users, 3)

	// Verify all users are returned
	emails := make(map[string]bool)
	for _, user := range users {
		emails[user.Email] = true
	}
	suite.Assert().True(emails["user1@test.com"])
	suite.Assert().True(emails["user2@test.com"])
	suite.Assert().True(emails["user3@test.com"])
}

// TestUpdateLastLogin tests updating last login timestamp
func (suite *UserRepositoryTestSuite) TestUpdateLastLogin() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Update last login
	loginTime := time.Now()
	err := suite.repository.UpdateLastLogin(suite.ctx, user.ID, loginTime)
	suite.Assert().NoError(err)

	// Verify update
	retrievedUser, err := suite.repository.GetByID(suite.ctx, user.ID)
	suite.Assert().NoError(err)
	suite.Assert().NotNil(retrievedUser.LastLoginAt)
	suite.Assert().WithinDuration(loginTime, *retrievedUser.LastLoginAt, time.Second)
}

// TestFailedLoginAttempts tests failed login attempt tracking
func (suite *UserRepositoryTestSuite) TestFailedLoginAttempts() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Increment failed login attempts
	err := suite.repository.IncrementFailedLoginAttempts(suite.ctx, user.ID)
	suite.Assert().NoError(err)

	// Verify increment
	var dbUser models.User
	err = suite.db.Where("id = ?", user.ID).First(&dbUser).Error
	suite.Assert().NoError(err)
	suite.Assert().Equal(1, dbUser.FailedLoginAttempts)

	// Reset failed login attempts
	err = suite.repository.ResetFailedLoginAttempts(suite.ctx, user.ID)
	suite.Assert().NoError(err)

	// Verify reset
	err = suite.db.Where("id = ?", user.ID).First(&dbUser).Error
	suite.Assert().NoError(err)
	suite.Assert().Equal(0, dbUser.FailedLoginAttempts)
}

// TestLockUser tests user account locking
func (suite *UserRepositoryTestSuite) TestLockUser() {
	// Create test data
	_, user := suite.createTestUserWithOrg()

	// Lock user
	lockUntil := time.Now().Add(30 * time.Minute)
	err := suite.repository.LockUser(suite.ctx, user.ID, lockUntil)
	suite.Assert().NoError(err)

	// Verify lock
	var dbUser models.User
	err = suite.db.Where("id = ?", user.ID).First(&dbUser).Error
	suite.Assert().NoError(err)
	suite.Assert().NotNil(dbUser.LockedUntil)
	suite.Assert().WithinDuration(lockUntil, *dbUser.LockedUntil, time.Second)
}

// Helper methods

func (suite *UserRepositoryTestSuite) createTestOrg() *models.Organization {
	// Create organization without Settings field for SQLite compatibility
	err := suite.db.Exec("INSERT INTO organizations (id, name, domain, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		uuid.New().String(), "Test Organization", "test.com", true, time.Now(), time.Now()).Error
	suite.Require().NoError(err)
	
	var org models.Organization
	err = suite.db.Where("domain = ?", "test.com").First(&org).Error
	suite.Require().NoError(err)
	return &org
}

func (suite *UserRepositoryTestSuite) createTestUser(orgID uuid.UUID, email string) *models.User {
	user := &models.User{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   "hashedpassword",
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *UserRepositoryTestSuite) createTestUserWithOrg() (*models.Organization, *models.User) {
	org := suite.createTestOrg()
	user := suite.createTestUser(org.ID, "test@test.com")
	return org, user
}

// TestUserRepository runs the user repository test suite
func TestUserRepository(t *testing.T) {
	suite.Run(t, new(UserRepositoryTestSuite))
}