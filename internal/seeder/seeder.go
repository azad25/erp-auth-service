package seeder

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"erp-auth-service/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Seeder struct {
	db *gorm.DB
}

func NewSeeder(db *gorm.DB) *Seeder {
	return &Seeder{db: db}
}

func (s *Seeder) SeedAll() error {
	log.Println("🌱 Starting database seeding...")

	// Check if data already exists
	var userCount int64
	s.db.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		log.Printf("⚠️  Database already contains %d users. Skipping seeding.", userCount)
		return nil
	}

	// Seed in order due to foreign key dependencies
	organizations, err := s.seedOrganizations()
	if err != nil {
		return fmt.Errorf("failed to seed organizations: %w", err)
	}

	permissions, err := s.seedPermissions()
	if err != nil {
		return fmt.Errorf("failed to seed permissions: %w", err)
	}

	roles, err := s.seedRoles(organizations)
	if err != nil {
		return fmt.Errorf("failed to seed roles: %w", err)
	}

	if err := s.seedRolePermissions(roles, permissions); err != nil {
		return fmt.Errorf("failed to seed role permissions: %w", err)
	}

	users, err := s.seedUsers(organizations)
	if err != nil {
		return fmt.Errorf("failed to seed users: %w", err)
	}

	if err := s.seedUserRoles(users, roles); err != nil {
		return fmt.Errorf("failed to seed user roles: %w", err)
	}

	log.Println("✅ Database seeding completed successfully!")
	s.printSeedingSummary()
	return nil
}

func (s *Seeder) seedOrganizations() ([]models.Organization, error) {
	log.Println("📊 Seeding organizations...")

	organizations := []models.Organization{
		{
			Name:   "TechCorp Solutions",
			Domain: "techcorp.com",
			Settings: models.Settings{
				Timezone:         "UTC",
				DateFormat:       "YYYY-MM-DD",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: true,
				SessionTimeout:   3600,
				CustomFields:     map[string]string{"industry": "Technology"},
			},
			IsActive: true,
		},
		{
			Name:   "Global Manufacturing Inc",
			Domain: "globalmanuf.com",
			Settings: models.Settings{
				Timezone:         "America/New_York",
				DateFormat:       "MM/DD/YYYY",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: false,
				SessionTimeout:   7200,
				CustomFields:     map[string]string{"industry": "Manufacturing"},
			},
			IsActive: true,
		},
		{
			Name:   "Healthcare Partners",
			Domain: "healthpartners.org",
			Settings: models.Settings{
				Timezone:         "America/Chicago",
				DateFormat:       "DD-MM-YYYY",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: true,
				SessionTimeout:   1800,
				CustomFields:     map[string]string{"industry": "Healthcare"},
			},
			IsActive: true,
		},
		{
			Name:   "EduTech Academy",
			Domain: "edutech.edu",
			Settings: models.Settings{
				Timezone:         "America/Los_Angeles",
				DateFormat:       "YYYY-MM-DD",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: true,
				SessionTimeout:   14400,
				CustomFields:     map[string]string{"industry": "Education"},
			},
			IsActive: true,
		},
		{
			Name:   "Financial Services Group",
			Domain: "finservices.com",
			Settings: models.Settings{
				Timezone:         "Europe/London",
				DateFormat:       "DD/MM/YYYY",
				Currency:         "GBP",
				Language:         "en",
				TwoFactorEnabled: true,
				SessionTimeout:   900,
				CustomFields:     map[string]string{"industry": "Finance"},
			},
			IsActive: true,
		},
		{
			Name:   "Retail Chain Ltd",
			Domain: "retailchain.com",
			Settings: models.Settings{
				Timezone:         "America/New_York",
				DateFormat:       "MM-DD-YYYY",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: false,
				SessionTimeout:   3600,
				CustomFields:     map[string]string{"industry": "Retail"},
			},
			IsActive: true,
		},
		{
			Name:   "Construction Pro",
			Domain: "constructionpro.com",
			Settings: models.Settings{
				Timezone:         "America/Denver",
				DateFormat:       "YYYY/MM/DD",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: false,
				SessionTimeout:   7200,
				CustomFields:     map[string]string{"industry": "Construction"},
			},
			IsActive: true,
		},
		{
			Name:   "Media & Entertainment Co",
			Domain: "mediaent.com",
			Settings: models.Settings{
				Timezone:         "America/Los_Angeles",
				DateFormat:       "MM/DD/YYYY",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: true,
				SessionTimeout:   10800,
				CustomFields:     map[string]string{"industry": "Media"},
			},
			IsActive: true,
		},
		{
			Name:   "Logistics Express",
			Domain: "logisticsexp.com",
			Settings: models.Settings{
				Timezone:         "UTC",
				DateFormat:       "DD-MM-YYYY",
				Currency:         "EUR",
				Language:         "en",
				TwoFactorEnabled: false,
				SessionTimeout:   5400,
				CustomFields:     map[string]string{"industry": "Logistics"},
			},
			IsActive: true,
		},
		{
			Name:   "Consulting Partners",
			Domain: "consultingpartners.com",
			Settings: models.Settings{
				Timezone:         "Europe/Berlin",
				DateFormat:       "DD.MM.YYYY",
				Currency:         "EUR",
				Language:         "en",
				TwoFactorEnabled: true,
				SessionTimeout:   7200,
				CustomFields:     map[string]string{"industry": "Consulting"},
			},
			IsActive: true,
		},
	}

	if err := s.db.Create(&organizations).Error; err != nil {
		return nil, err
	}

	log.Printf("✅ Created %d organizations", len(organizations))
	return organizations, nil
}

func (s *Seeder) seedPermissions() ([]models.Permission, error) {
	log.Println("🔐 Seeding permissions...")

	permissions := []models.Permission{
		// User Management
		{Name: "users.create", Resource: "users", Action: "create", Description: "Create new users"},
		{Name: "users.read", Resource: "users", Action: "read", Description: "View user information"},
		{Name: "users.update", Resource: "users", Action: "update", Description: "Update user information"},
		{Name: "users.delete", Resource: "users", Action: "delete", Description: "Delete users"},
		{Name: "users.list", Resource: "users", Action: "list", Description: "List all users"},

		// Role Management
		{Name: "roles.create", Resource: "roles", Action: "create", Description: "Create new roles"},
		{Name: "roles.read", Resource: "roles", Action: "read", Description: "View role information"},
		{Name: "roles.update", Resource: "roles", Action: "update", Description: "Update role information"},
		{Name: "roles.delete", Resource: "roles", Action: "delete", Description: "Delete roles"},
		{Name: "roles.list", Resource: "roles", Action: "list", Description: "List all roles"},
		{Name: "roles.assign", Resource: "roles", Action: "assign", Description: "Assign roles to users"},

		// Organization Management
		{Name: "organizations.create", Resource: "organizations", Action: "create", Description: "Create organizations"},
		{Name: "organizations.read", Resource: "organizations", Action: "read", Description: "View organization information"},
		{Name: "organizations.update", Resource: "organizations", Action: "update", Description: "Update organization settings"},
		{Name: "organizations.delete", Resource: "organizations", Action: "delete", Description: "Delete organizations"},

		// CRM Module
		{Name: "crm.contacts.create", Resource: "crm.contacts", Action: "create", Description: "Create CRM contacts"},
		{Name: "crm.contacts.read", Resource: "crm.contacts", Action: "read", Description: "View CRM contacts"},
		{Name: "crm.contacts.update", Resource: "crm.contacts", Action: "update", Description: "Update CRM contacts"},
		{Name: "crm.contacts.delete", Resource: "crm.contacts", Action: "delete", Description: "Delete CRM contacts"},
		{Name: "crm.leads.manage", Resource: "crm.leads", Action: "manage", Description: "Manage CRM leads"},
		{Name: "crm.opportunities.manage", Resource: "crm.opportunities", Action: "manage", Description: "Manage sales opportunities"},

		// HRM Module
		{Name: "hrm.employees.create", Resource: "hrm.employees", Action: "create", Description: "Create employee records"},
		{Name: "hrm.employees.read", Resource: "hrm.employees", Action: "read", Description: "View employee information"},
		{Name: "hrm.employees.update", Resource: "hrm.employees", Action: "update", Description: "Update employee information"},
		{Name: "hrm.employees.delete", Resource: "hrm.employees", Action: "delete", Description: "Delete employee records"},
		{Name: "hrm.payroll.manage", Resource: "hrm.payroll", Action: "manage", Description: "Manage payroll"},
		{Name: "hrm.attendance.manage", Resource: "hrm.attendance", Action: "manage", Description: "Manage attendance"},

		// Finance Module
		{Name: "finance.accounts.create", Resource: "finance.accounts", Action: "create", Description: "Create financial accounts"},
		{Name: "finance.accounts.read", Resource: "finance.accounts", Action: "read", Description: "View financial accounts"},
		{Name: "finance.accounts.update", Resource: "finance.accounts", Action: "update", Description: "Update financial accounts"},
		{Name: "finance.transactions.create", Resource: "finance.transactions", Action: "create", Description: "Create financial transactions"},
		{Name: "finance.transactions.read", Resource: "finance.transactions", Action: "read", Description: "View financial transactions"},
		{Name: "finance.reports.view", Resource: "finance.reports", Action: "view", Description: "View financial reports"},

		// Inventory Module
		{Name: "inventory.products.create", Resource: "inventory.products", Action: "create", Description: "Create products"},
		{Name: "inventory.products.read", Resource: "inventory.products", Action: "read", Description: "View products"},
		{Name: "inventory.products.update", Resource: "inventory.products", Action: "update", Description: "Update products"},
		{Name: "inventory.products.delete", Resource: "inventory.products", Action: "delete", Description: "Delete products"},
		{Name: "inventory.stock.manage", Resource: "inventory.stock", Action: "manage", Description: "Manage stock levels"},

		// Project Management
		{Name: "projects.create", Resource: "projects", Action: "create", Description: "Create projects"},
		{Name: "projects.read", Resource: "projects", Action: "read", Description: "View projects"},
		{Name: "projects.update", Resource: "projects", Action: "update", Description: "Update projects"},
		{Name: "projects.delete", Resource: "projects", Action: "delete", Description: "Delete projects"},
		{Name: "projects.tasks.manage", Resource: "projects.tasks", Action: "manage", Description: "Manage project tasks"},

		// System Administration
		{Name: "system.settings.read", Resource: "system.settings", Action: "read", Description: "View system settings"},
		{Name: "system.settings.update", Resource: "system.settings", Action: "update", Description: "Update system settings"},
		{Name: "system.logs.read", Resource: "system.logs", Action: "read", Description: "View system logs"},
		{Name: "system.backup.manage", Resource: "system.backup", Action: "manage", Description: "Manage system backups"},

		// Reports and Analytics
		{Name: "reports.view", Resource: "reports", Action: "view", Description: "View reports"},
		{Name: "reports.create", Resource: "reports", Action: "create", Description: "Create custom reports"},
		{Name: "analytics.view", Resource: "analytics", Action: "view", Description: "View analytics dashboards"},
	}

	if err := s.db.Create(&permissions).Error; err != nil {
		return nil, err
	}

	log.Printf("✅ Created %d permissions", len(permissions))
	return permissions, nil
}

func (s *Seeder) seedRoles(organizations []models.Organization) ([]models.Role, error) {
	log.Println("👥 Seeding roles...")

	var allRoles []models.Role

	// Define role templates
	roleTemplates := []struct {
		Name        string
		Description string
		IsSystem    bool
	}{
		{"Super Admin", "Full system access with all permissions", true},
		{"Organization Admin", "Full access within organization", false},
		{"Manager", "Management level access with team oversight", false},
		{"Employee", "Standard employee access", false},
		{"HR Manager", "Human resources management access", false},
		{"Finance Manager", "Financial management access", false},
		{"Sales Manager", "Sales and CRM management access", false},
		{"Project Manager", "Project management access", false},
		{"Inventory Manager", "Inventory and stock management access", false},
		{"Viewer", "Read-only access to most resources", false},
	}

	// Create roles for each organization
	for _, org := range organizations {
		for _, template := range roleTemplates {
			role := models.Role{
				OrganizationID: org.ID,
				Name:           template.Name,
				Description:    template.Description,
				IsSystem:       template.IsSystem,
				IsActive:       true,
			}
			allRoles = append(allRoles, role)
		}
	}

	if err := s.db.Create(&allRoles).Error; err != nil {
		return nil, err
	}

	log.Printf("✅ Created %d roles", len(allRoles))
	return allRoles, nil
}

func (s *Seeder) seedRolePermissions(roles []models.Role, permissions []models.Permission) error {
	log.Println("🔗 Seeding role permissions...")

	var rolePermissions []models.RolePermission

	// Create permission maps for easier lookup
	permissionMap := make(map[string]models.Permission)
	for _, perm := range permissions {
		permissionMap[perm.Name] = perm
	}

	// Define role permission mappings
	rolePermissionMap := map[string][]string{
		"Super Admin": {
			// All permissions for super admin
		},
		"Organization Admin": {
			"users.create", "users.read", "users.update", "users.delete", "users.list",
			"roles.create", "roles.read", "roles.update", "roles.delete", "roles.list", "roles.assign",
			"organizations.read", "organizations.update",
			"system.settings.read", "system.settings.update",
			"reports.view", "reports.create", "analytics.view",
		},
		"Manager": {
			"users.read", "users.list",
			"roles.read", "roles.list",
			"crm.contacts.create", "crm.contacts.read", "crm.contacts.update",
			"crm.leads.manage", "crm.opportunities.manage",
			"hrm.employees.read", "hrm.attendance.manage",
			"projects.create", "projects.read", "projects.update", "projects.tasks.manage",
			"reports.view", "analytics.view",
		},
		"Employee": {
			"users.read",
			"crm.contacts.read",
			"hrm.employees.read",
			"projects.read", "projects.tasks.manage",
			"inventory.products.read",
			"reports.view",
		},
		"HR Manager": {
			"users.create", "users.read", "users.update", "users.list",
			"hrm.employees.create", "hrm.employees.read", "hrm.employees.update", "hrm.employees.delete",
			"hrm.payroll.manage", "hrm.attendance.manage",
			"reports.view",
		},
		"Finance Manager": {
			"finance.accounts.create", "finance.accounts.read", "finance.accounts.update",
			"finance.transactions.create", "finance.transactions.read",
			"finance.reports.view",
			"reports.view", "reports.create",
		},
		"Sales Manager": {
			"crm.contacts.create", "crm.contacts.read", "crm.contacts.update", "crm.contacts.delete",
			"crm.leads.manage", "crm.opportunities.manage",
			"reports.view", "analytics.view",
		},
		"Project Manager": {
			"projects.create", "projects.read", "projects.update", "projects.delete",
			"projects.tasks.manage",
			"reports.view",
		},
		"Inventory Manager": {
			"inventory.products.create", "inventory.products.read", "inventory.products.update", "inventory.products.delete",
			"inventory.stock.manage",
			"reports.view",
		},
		"Viewer": {
			"users.read",
			"crm.contacts.read",
			"hrm.employees.read",
			"finance.accounts.read", "finance.transactions.read",
			"inventory.products.read",
			"projects.read",
			"reports.view",
		},
	}

	// Super Admin gets all permissions
	allPermissionNames := make([]string, len(permissions))
	for i, perm := range permissions {
		allPermissionNames[i] = perm.Name
	}
	rolePermissionMap["Super Admin"] = allPermissionNames

	// Create role permissions
	for _, role := range roles {
		if permNames, exists := rolePermissionMap[role.Name]; exists {
			for _, permName := range permNames {
				if perm, permExists := permissionMap[permName]; permExists {
					rolePermissions = append(rolePermissions, models.RolePermission{
						RoleID:       role.ID,
						PermissionID: perm.ID,
					})
				}
			}
		}
	}

	if err := s.db.Create(&rolePermissions).Error; err != nil {
		return err
	}

	log.Printf("✅ Created %d role permissions", len(rolePermissions))
	return nil
}

func (s *Seeder) seedUsers(organizations []models.Organization) ([]models.User, error) {
	log.Println("👤 Seeding users...")

	// Sample user data
	firstNames := []string{
		"John", "Jane", "Michael", "Sarah", "David", "Emily", "Robert", "Lisa", "William", "Jennifer",
		"James", "Mary", "Christopher", "Patricia", "Daniel", "Linda", "Matthew", "Elizabeth", "Anthony", "Barbara",
		"Mark", "Susan", "Donald", "Jessica", "Steven", "Karen", "Paul", "Nancy", "Andrew", "Betty",
		"Joshua", "Helen", "Kenneth", "Sandra", "Kevin", "Donna", "Brian", "Carol", "George", "Ruth",
		"Edward", "Sharon", "Ronald", "Michelle", "Timothy", "Laura", "Jason", "Sarah", "Jeffrey", "Kimberly",
		"Ryan", "Deborah", "Jacob", "Dorothy", "Gary", "Lisa", "Nicholas", "Nancy", "Eric", "Karen",
		"Jonathan", "Betty", "Stephen", "Helen", "Larry", "Sandra", "Justin", "Donna", "Scott", "Carol",
		"Brandon", "Ruth", "Benjamin", "Sharon", "Samuel", "Michelle", "Gregory", "Laura", "Alexander", "Sarah",
		"Patrick", "Kimberly", "Jack", "Deborah", "Dennis", "Dorothy", "Jerry", "Lisa", "Tyler", "Nancy",
		"Aaron", "Karen", "Jose", "Betty", "Henry", "Helen", "Adam", "Sandra", "Douglas", "Donna",
	}

	lastNames := []string{
		"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez",
		"Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas", "Taylor", "Moore", "Jackson", "Martin",
		"Lee", "Perez", "Thompson", "White", "Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson",
		"Walker", "Young", "Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
		"Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell", "Carter", "Roberts",
		"Gomez", "Phillips", "Evans", "Turner", "Diaz", "Parker", "Cruz", "Edwards", "Collins", "Reyes",
		"Stewart", "Morris", "Morales", "Murphy", "Cook", "Rogers", "Gutierrez", "Ortiz", "Morgan", "Cooper",
		"Peterson", "Bailey", "Reed", "Kelly", "Howard", "Ramos", "Kim", "Cox", "Ward", "Richardson",
		"Watson", "Brooks", "Chavez", "Wood", "James", "Bennett", "Gray", "Mendoza", "Ruiz", "Hughes",
		"Price", "Alvarez", "Castillo", "Sanders", "Patel", "Myers", "Long", "Ross", "Foster", "Jimenez",
	}

	domains := []string{"gmail.com", "yahoo.com", "hotmail.com", "outlook.com", "company.com"}

	var users []models.User
	rand.Seed(time.Now().UnixNano())

	// Create 100 users distributed across organizations
	usersPerOrg := 100 / len(organizations)
	remainder := 100 % len(organizations)

	userIndex := 0
	for orgIndex, org := range organizations {
		// Calculate how many users for this organization
		numUsers := usersPerOrg
		if orgIndex < remainder {
			numUsers++
		}

		for i := 0; i < numUsers; i++ {
			firstName := firstNames[rand.Intn(len(firstNames))]
			lastName := lastNames[rand.Intn(len(lastNames))]
			domain := domains[rand.Intn(len(domains))]
			
			// Create unique email
			email := fmt.Sprintf("%s.%s%d@%s", 
				strings.ToLower(firstName), 
				strings.ToLower(lastName), 
				userIndex+1, 
				domain)

			user := models.User{
				OrganizationID: org.ID,
				Email:          email,
				FirstName:      firstName,
				LastName:       lastName,
				IsActive:       rand.Float32() > 0.1, // 90% active users
				IsVerified:     rand.Float32() > 0.2, // 80% verified users
				TwoFactorEnabled: rand.Float32() > 0.7, // 30% have 2FA enabled
			}

			// Set password (default: "password123")
			if err := user.SetPassword("password123"); err != nil {
				return nil, fmt.Errorf("failed to set password for user %s: %w", email, err)
			}

			// Set random last login time for some users
			if rand.Float32() > 0.3 { // 70% have logged in before
				lastLogin := time.Now().AddDate(0, 0, -rand.Intn(30)) // Within last 30 days
				user.LastLoginAt = &lastLogin
			}

			users = append(users, user)
			userIndex++
		}
	}

	if err := s.db.Create(&users).Error; err != nil {
		return nil, err
	}

	log.Printf("✅ Created %d users", len(users))
	return users, nil
}

func (s *Seeder) seedUserRoles(users []models.User, roles []models.Role) error {
	log.Println("🔗 Seeding user roles...")

	// Create role maps by organization for easier lookup
	rolesByOrg := make(map[uuid.UUID][]models.Role)
	for _, role := range roles {
		rolesByOrg[role.OrganizationID] = append(rolesByOrg[role.OrganizationID], role)
	}

	var userRoles []models.UserRole
	rand.Seed(time.Now().UnixNano())

	// Role distribution weights (higher number = more likely to be assigned)
	roleWeights := map[string]int{
		"Employee":           40, // Most common
		"Manager":            15,
		"Viewer":             15,
		"HR Manager":         5,
		"Finance Manager":    5,
		"Sales Manager":      5,
		"Project Manager":    5,
		"Inventory Manager":  5,
		"Organization Admin": 3,
		"Super Admin":        2, // Least common
	}

	// Create weighted role list for random selection
	var weightedRoles []string
	for roleName, weight := range roleWeights {
		for i := 0; i < weight; i++ {
			weightedRoles = append(weightedRoles, roleName)
		}
	}

	// Assign roles to users
	for _, user := range users {
		orgRoles := rolesByOrg[user.OrganizationID]
		if len(orgRoles) == 0 {
			continue
		}

		// Select a random role based on weights
		selectedRoleName := weightedRoles[rand.Intn(len(weightedRoles))]
		
		// Find the actual role object
		var selectedRole *models.Role
		for _, role := range orgRoles {
			if role.Name == selectedRoleName {
				selectedRole = &role
				break
			}
		}

		if selectedRole != nil {
			userRoles = append(userRoles, models.UserRole{
				UserID: user.ID,
				RoleID: selectedRole.ID,
			})
		}

		// Some users might have multiple roles (10% chance)
		if rand.Float32() < 0.1 && len(orgRoles) > 1 {
			// Add a second role
			secondRoleName := weightedRoles[rand.Intn(len(weightedRoles))]
			for _, role := range orgRoles {
				if role.Name == secondRoleName && role.ID != selectedRole.ID {
					userRoles = append(userRoles, models.UserRole{
						UserID: user.ID,
						RoleID: role.ID,
					})
					break
				}
			}
		}
	}

	if err := s.db.Create(&userRoles).Error; err != nil {
		return err
	}

	log.Printf("✅ Created %d user role assignments", len(userRoles))
	return nil
}

func (s *Seeder) printSeedingSummary() {
	log.Println("\n📊 Seeding Summary:")
	log.Println("==================")

	// Count organizations
	var orgCount int64
	s.db.Model(&models.Organization{}).Count(&orgCount)
	log.Printf("Organizations: %d", orgCount)

	// Count users
	var userCount int64
	s.db.Model(&models.User{}).Count(&userCount)
	log.Printf("Users: %d", userCount)

	// Count active users
	var activeUserCount int64
	s.db.Model(&models.User{}).Where("is_active = ?", true).Count(&activeUserCount)
	log.Printf("Active Users: %d", activeUserCount)

	// Count verified users
	var verifiedUserCount int64
	s.db.Model(&models.User{}).Where("is_verified = ?", true).Count(&verifiedUserCount)
	log.Printf("Verified Users: %d", verifiedUserCount)

	// Count roles
	var roleCount int64
	s.db.Model(&models.Role{}).Count(&roleCount)
	log.Printf("Roles: %d", roleCount)

	// Count permissions
	var permissionCount int64
	s.db.Model(&models.Permission{}).Count(&permissionCount)
	log.Printf("Permissions: %d", permissionCount)

	// Count user roles
	var userRoleCount int64
	s.db.Model(&models.UserRole{}).Count(&userRoleCount)
	log.Printf("User Role Assignments: %d", userRoleCount)

	// Count role permissions
	var rolePermissionCount int64
	s.db.Model(&models.RolePermission{}).Count(&rolePermissionCount)
	log.Printf("Role Permission Assignments: %d", rolePermissionCount)

	log.Println("\n🔐 Default Login Credentials:")
	log.Println("=============================")
	log.Println("All users have the password: 'password123'")
	log.Println("Example users:")
	
	// Show some example users from each organization
	var sampleUsers []models.User
	s.db.Preload("Organization").Limit(5).Find(&sampleUsers)
	
	for _, user := range sampleUsers {
		log.Printf("  • %s (%s) - %s", user.Email, user.GetFullName(), user.Organization.Name)
	}
	
	log.Println("\n✅ Seeding completed successfully!")
}

// SeedMinimal creates minimal data for testing
func (s *Seeder) SeedMinimal() error {
	log.Println("🌱 Starting minimal database seeding...")

	// Create one organization
	org := models.Organization{
		Name:   "Test Organization",
		Domain: "test.com",
		Settings: models.Settings{
			Timezone:         "UTC",
			DateFormat:       "YYYY-MM-DD",
			Currency:         "USD",
			Language:         "en",
			TwoFactorEnabled: false,
			SessionTimeout:   3600,
			CustomFields:     map[string]string{"type": "test"},
		},
		IsActive: true,
	}

	if err := s.db.Create(&org).Error; err != nil {
		return fmt.Errorf("failed to create test organization: %w", err)
	}

	// Create basic permissions
	permissions := []models.Permission{
		{Name: "users.read", Resource: "users", Action: "read", Description: "View users"},
		{Name: "users.create", Resource: "users", Action: "create", Description: "Create users"},
		{Name: "roles.read", Resource: "roles", Action: "read", Description: "View roles"},
	}

	if err := s.db.Create(&permissions).Error; err != nil {
		return fmt.Errorf("failed to create permissions: %w", err)
	}

	// Create basic roles
	adminRole := models.Role{
		OrganizationID: org.ID,
		Name:           "Admin",
		Description:    "Administrator role",
		IsSystem:       true,
		IsActive:       true,
	}

	userRole := models.Role{
		OrganizationID: org.ID,
		Name:           "User",
		Description:    "Standard user role",
		IsSystem:       false,
		IsActive:       true,
	}

	roles := []models.Role{adminRole, userRole}
	if err := s.db.Create(&roles).Error; err != nil {
		return fmt.Errorf("failed to create roles: %w", err)
	}

	// Create test users
	adminUser := models.User{
		OrganizationID: org.ID,
		Email:          "admin@test.com",
		FirstName:      "Admin",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
	}
	adminUser.SetPassword("admin123")

	testUser := models.User{
		OrganizationID: org.ID,
		Email:          "user@test.com",
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
	}
	testUser.SetPassword("user123")

	users := []models.User{adminUser, testUser}
	if err := s.db.Create(&users).Error; err != nil {
		return fmt.Errorf("failed to create users: %w", err)
	}

	// Assign roles to users
	userRoles := []models.UserRole{
		{UserID: adminUser.ID, RoleID: adminRole.ID},
		{UserID: testUser.ID, RoleID: userRole.ID},
	}

	if err := s.db.Create(&userRoles).Error; err != nil {
		return fmt.Errorf("failed to assign user roles: %w", err)
	}

	log.Println("✅ Minimal seeding completed!")
	log.Println("Test credentials:")
	log.Println("  Admin: admin@test.com / admin123")
	log.Println("  User:  user@test.com / user123")

	return nil
}