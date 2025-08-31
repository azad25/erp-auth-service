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
	"gorm.io/gorm/clause"
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

	if err := s.seedUserActivities(users); err != nil {
		return fmt.Errorf("failed to seed user activities: %w", err)
	}

	log.Println("✅ Database seeding completed successfully!")
	s.printSeedingSummary()
	return nil
}

func (s *Seeder) seedOrganizations() ([]models.Organization, error) {
	log.Println("📊 Seeding organizations...")

	// Create default organization with predictable UUID for AI Copilot service
	defaultOrgID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	organizations := []models.Organization{
		{
			ID:     defaultOrgID,
			Name:   "Default Organization",
			Domain: "default.local",
			Settings: models.Settings{
				Timezone:         "UTC",
				DateFormat:       "YYYY-MM-DD",
				Currency:         "USD",
				Language:         "en",
				TwoFactorEnabled: false,
				SessionTimeout:   3600,
				CustomFields:     map[string]string{"type": "default"},
			},
			IsActive: true,
		},
		{
			Name:   "UniBASE ERP Solutions",
			Domain: "unibaseerp.com",
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

	// Build list of domains for lookup
	domains := make([]string, 0, len(organizations))
	for _, org := range organizations {
		domains = append(domains, org.Domain)
	}

	// Idempotent insert: ON CONFLICT(domain) DO NOTHING
	if err := s.db.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "domain"}},
			DoNothing: true,
		}).
		Create(&organizations).Error; err != nil {
		return nil, err
	}

	// Reload organizations (existing or newly inserted) to get IDs
	var out []models.Organization
	if err := s.db.Where("domain IN ?", domains).Find(&out).Error; err != nil {
		return nil, err
	}

	log.Printf("✅ Ensured %d organizations (inserted or existing)", len(out))
	return out, nil
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
		{Name: "organizations.read_all", Resource: "organizations", Action: "read_all", Description: "View all organizations (app admin only)"},
		{Name: "organizations.update", Resource: "organizations", Action: "update", Description: "Update organization settings"},
		{Name: "organizations.delete", Resource: "organizations", Action: "delete", Description: "Delete organizations"},

		// User Management Permissions
		{Name: "users.manage", Resource: "users", Action: "manage", Description: "Manage users within organization"},
		{Name: "users.read_all", Resource: "users", Action: "read_all", Description: "View all users across organizations (app admin only)"},

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

	// Idempotent insert on unique(name)
	if err := s.db.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoNothing: true,
		}).
		Create(&permissions).Error; err != nil {
		return nil, err
	}

	// Reload full permission records by name to ensure IDs are populated
	var out []models.Permission
	names := make([]string, 0, len(permissions))
	for _, p := range permissions {
		names = append(names, p.Name)
	}
	if err := s.db.Where("name IN ?", names).Find(&out).Error; err != nil {
		return nil, err
	}

	log.Printf("✅ Ensured %d permissions (inserted or existing)", len(out))
	return out, nil
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

	// Ensure roles exist for each organization without creating duplicates
	for _, org := range organizations {
		for _, template := range roleTemplates {
			var role models.Role
			if err := s.db.
				Attrs(models.Role{
					Description: template.Description,
					IsSystem:    template.IsSystem,
					IsActive:    true,
				}).
				FirstOrCreate(&role, models.Role{OrganizationID: org.ID, Name: template.Name}).Error; err != nil {
				return nil, err
			}
			allRoles = append(allRoles, role)
		}
	}

	log.Printf("✅ Ensured %d roles (created or existing)", len(allRoles))
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
			// All permissions for super admin (app admin)
			"users.create", "users.read", "users.update", "users.delete", "users.list", "users.manage", "users.read_all",
			"roles.create", "roles.read", "roles.update", "roles.delete", "roles.list", "roles.assign",
			"organizations.create", "organizations.read", "organizations.read_all", "organizations.update", "organizations.delete", "organizations.list", "organizations.manage",
			"system.settings.read", "system.settings.update", "system.logs.read", "system.backup.manage",
			"reports.view", "reports.create", "analytics.view",
			"crm.contacts.create", "crm.contacts.read", "crm.contacts.update", "crm.contacts.delete",
			"crm.leads.manage", "crm.opportunities.manage",
			"hrm.employees.create", "hrm.employees.read", "hrm.employees.update", "hrm.employees.delete",
			"hrm.payroll.manage", "hrm.attendance.manage",
			"finance.accounts.create", "finance.accounts.read", "finance.accounts.update",
			"finance.transactions.create", "finance.transactions.read", "finance.reports.view",
			"inventory.products.create", "inventory.products.read", "inventory.products.update", "inventory.products.delete",
			"inventory.stock.manage",
			"projects.create", "projects.read", "projects.update", "projects.delete", "projects.tasks.manage",
		},
		"Organization Admin": {
			"users.create", "users.read", "users.update", "users.delete", "users.list", "users.manage",
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

	if err := s.db.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "role_id"}, {Name: "permission_id"}},
			DoNothing: true,
		}).
		Create(&rolePermissions).Error; err != nil {
		return err
	}

	log.Printf("✅ Ensured %d role permissions (created or existing)", len(rolePermissions))
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
	// No need to seed random as of Go 1.20+

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
				OrganizationID:   org.ID,
				Email:            email,
				FirstName:        firstName,
				LastName:         lastName,
				IsActive:         rand.Float32() > 0.1, // 90% active users
				IsVerified:       rand.Float32() > 0.2, // 80% verified users
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

		if org.Domain == "unibaseerp.com" {
			adminUser := models.User{
				OrganizationID: org.ID,
				Email:          "admin@unibaseerp.com",
				FirstName:      "Admin",
				LastName:       "User",
				IsActive:       true,
				IsVerified:     true,
			}
			adminUser.SetPassword("admin123")
			users = append(users, adminUser)
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
			// Special handling for admin user - always assign Super Admin role
			if user.Email == "admin@unibaseerp.com" {
				// Find Super Admin role for this organization
				var superAdminRole *models.Role
				for _, role := range orgRoles {
					if role.Name == "Super Admin" {
						superAdminRole = &role
						break
					}
				}
				if superAdminRole != nil {
					userRoles = append(userRoles, models.UserRole{
						UserID: user.ID,
						RoleID: superAdminRole.ID,
					})
				}
			} else {
				// For regular users, assign the selected role
				userRoles = append(userRoles, models.UserRole{
					UserID: user.ID,
					RoleID: selectedRole.ID,
				})
			}
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

	if err := s.db.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "role_id"}},
			DoNothing: true,
		}).
		Create(&userRoles).Error; err != nil {
		return err
	}

	log.Printf("✅ Ensured %d user role assignments (created or existing)", len(userRoles))
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

	// Count user activities
	var activityCount int64
	s.db.Model(&models.UserActivity{}).Count(&activityCount)
	log.Printf("User Activities: %d", activityCount)

	log.Println("\n🔐 Default Login Credentials:")
	log.Println("=============================")
	log.Println("All users have the password: 'password123'")
	log.Println("Example users:")

	// Show super admin credentials
	log.Println("  • Super Admin:")
	log.Println("    - Email: admin@unibaseerp.com")
	log.Println("    - Password: admin123")

	// Show some example users from each organization
	var sampleUsers []models.User
	s.db.Preload("Organization").Limit(5).Find(&sampleUsers)

	for _, user := range sampleUsers {
		log.Printf("  • %s (%s) - %s", user.Email, user.GetFullName(), user.Organization.Name)
	}

	log.Println("\n✅ Seeding completed successfully!")
}
func (s *Seeder) seedUserActivities(users []models.User) error {
	log.Println("📊 Seeding user activities...")

	// Check if activities already exist
	var activityCount int64
	s.db.Model(&models.UserActivity{}).Count(&activityCount)
	if activityCount > 0 {
		log.Printf("⚠️  Database already contains %d activities. Skipping activity seeding.", activityCount)
		return nil
	}

	var activities []models.UserActivity

	// Activity types and resources
	activityTypes := []struct {
		Action   string
		Resource string
		Details  string
	}{
		{models.ActionLogin, models.ResourceAuth, `{"success": true, "method": "email_password"}`},
		{models.ActionLogout, models.ResourceAuth, `{"session_duration": 3600}`},
		{models.ActionLoginFailed, models.ResourceAuth, `{"reason": "invalid_password", "attempts": 1}`},
		{models.ActionPasswordChanged, models.ResourceProfile, `{"strength": "strong"}`},
		{models.ActionProfileUpdated, models.ResourceProfile, `{"fields": ["first_name", "last_name"]}`},
		{models.ActionUserCreated, models.ResourceUser, `{"role": "employee"}`},
		{models.ActionUserUpdated, models.ResourceUser, `{"fields": ["email", "is_active"]}`},
		{models.ActionUserActivated, models.ResourceUser, `{"previous_status": "inactive"}`},
		{models.ActionUserDeactivated, models.ResourceUser, `{"reason": "policy_violation"}`},
		{models.ActionRoleAssigned, models.ResourceRole, `{"role_name": "manager"}`},
		{models.ActionRoleRemoved, models.ResourceRole, `{"role_name": "employee"}`},
		{models.ActionOrganizationUpdated, models.ResourceOrganization, `{"fields": ["settings"]}`},
	}

	// IP addresses for variety
	ipAddresses := []string{
		"192.168.1.100", "10.0.0.50", "172.16.0.25", "203.0.113.10", "198.51.100.5",
		"192.168.0.150", "10.1.1.75", "172.20.0.30", "203.0.113.20", "198.51.100.15",
	}

	// User agents for variety
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:89.0) Gecko/20100101 Firefox/89.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.1 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 14_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Android 11; Mobile; rv:68.0) Gecko/68.0 Firefox/88.0",
	}

	// Generate activities for each user
	for _, user := range users {
		// Generate 5-20 activities per user
		numActivities := rand.Intn(16) + 5

		for i := 0; i < numActivities; i++ {
			// Select random activity type
			activityType := activityTypes[rand.Intn(len(activityTypes))]
			
			// Create activity with random timestamp in the last 30 days
			createdAt := time.Now().AddDate(0, 0, -rand.Intn(30)).Add(
				-time.Duration(rand.Intn(24)) * time.Hour,
			).Add(
				-time.Duration(rand.Intn(60)) * time.Minute,
			)

			activity := models.UserActivity{
				UserID:         user.ID,
				OrganizationID: user.OrganizationID,
				Action:         activityType.Action,
				Resource:       activityType.Resource,
				Details:        activityType.Details,
				IPAddress:      ipAddresses[rand.Intn(len(ipAddresses))],
				UserAgent:      userAgents[rand.Intn(len(userAgents))],
				CreatedAt:      createdAt,
				UpdatedAt:      createdAt,
			}

			activities = append(activities, activity)
		}

		// Add some recent login activities for active users
		if user.IsActive && user.LastLoginAt != nil {
			// Add a recent successful login
			recentLogin := models.UserActivity{
				UserID:         user.ID,
				OrganizationID: user.OrganizationID,
				Action:         models.ActionLogin,
				Resource:       models.ResourceAuth,
				Details:        `{"success": true, "method": "email_password", "remember_me": false}`,
				IPAddress:      ipAddresses[rand.Intn(len(ipAddresses))],
				UserAgent:      userAgents[rand.Intn(len(userAgents))],
				CreatedAt:      *user.LastLoginAt,
				UpdatedAt:      *user.LastLoginAt,
			}
			activities = append(activities, recentLogin)
		}
	}

	// Add some failed login attempts (security events)
	for i := 0; i < 20; i++ {
		// Random user for failed login attempt
		user := users[rand.Intn(len(users))]
		
		failedLogin := models.UserActivity{
			UserID:         user.ID, // Use actual user ID instead of nil
			OrganizationID: user.OrganizationID,
			Action:         models.ActionLoginFailed,
			Resource:       models.ResourceAuth,
			Details:        fmt.Sprintf(`{"email": "%s", "reason": "invalid_password", "attempts": %d}`, user.Email, rand.Intn(3)+1),
			IPAddress:      ipAddresses[rand.Intn(len(ipAddresses))],
			UserAgent:      userAgents[rand.Intn(len(userAgents))],
			CreatedAt:      time.Now().AddDate(0, 0, -rand.Intn(7)), // Within last week
			UpdatedAt:      time.Now().AddDate(0, 0, -rand.Intn(7)),
		}
		activities = append(activities, failedLogin)
	}

	// Batch insert activities
	batchSize := 100
	for i := 0; i < len(activities); i += batchSize {
		end := i + batchSize
		if end > len(activities) {
			end = len(activities)
		}

		if err := s.db.Create(activities[i:end]).Error; err != nil {
			return fmt.Errorf("failed to create activity batch: %w", err)
		}
	}

	log.Printf("✅ Created %d user activities", len(activities))
	return nil
}