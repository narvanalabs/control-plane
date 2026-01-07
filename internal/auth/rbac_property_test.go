package auth

import (
	"context"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/narvanalabs/control-plane/internal/store"
)

// **Feature: platform-enhancements, Property 9: Owner Registration Blocks Public Signup**
// For any system state where an Owner user exists, attempting to register a new user
// without an invitation SHALL be rejected.
// **Validates: Requirements 14.2**

// mockUserStoreRBAC is a simple in-memory implementation for testing.
type mockUserStoreRBAC struct {
	users []*store.User
}

func (m *mockUserStoreRBAC) Create(ctx context.Context, email, password string, isAdmin bool) (*store.User, error) {
	return nil, nil
}

func (m *mockUserStoreRBAC) CreateWithRole(ctx context.Context, email, password string, role store.Role, invitedBy string) (*store.User, error) {
	return nil, nil
}

func (m *mockUserStoreRBAC) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	return nil, nil
}

func (m *mockUserStoreRBAC) GetByID(ctx context.Context, id string) (*store.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUserStoreRBAC) Authenticate(ctx context.Context, email, password string) (*store.User, error) {
	return nil, nil
}

func (m *mockUserStoreRBAC) Update(ctx context.Context, user *store.User) error {
	return nil
}

func (m *mockUserStoreRBAC) List(ctx context.Context) ([]*store.User, error) {
	return m.users, nil
}

func (m *mockUserStoreRBAC) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockUserStoreRBAC) CountByRole(ctx context.Context, role store.Role) (int, error) {
	count := 0
	for _, u := range m.users {
		if u.Role == role {
			count++
		}
	}
	return count, nil
}

func (m *mockUserStoreRBAC) GetFirstOwner(ctx context.Context) (*store.User, error) {
	for _, u := range m.users {
		if u.Role == store.RoleOwner {
			return u, nil
		}
	}
	return nil, nil
}

// mockStoreRBAC wraps mockUserStoreRBAC to implement store.Store interface partially.
type mockStoreRBAC struct {
	userStore *mockUserStoreRBAC
}

func (m *mockStoreRBAC) Users() store.UserStore {
	return m.userStore
}

// Stub implementations for other store methods (not used in these tests).
func (m *mockStoreRBAC) Orgs() store.OrgStore                                         { return nil }
func (m *mockStoreRBAC) Apps() store.AppStore                                         { return nil }
func (m *mockStoreRBAC) Deployments() store.DeploymentStore                           { return nil }
func (m *mockStoreRBAC) Nodes() store.NodeStore                                       { return nil }
func (m *mockStoreRBAC) Builds() store.BuildStore                                     { return nil }
func (m *mockStoreRBAC) Secrets() store.SecretStore                                   { return nil }
func (m *mockStoreRBAC) Logs() store.LogStore                                         { return nil }
func (m *mockStoreRBAC) GitHub() store.GitHubStore                                    { return nil }
func (m *mockStoreRBAC) GitHubAccounts() store.GitHubAccountStore                     { return nil }
func (m *mockStoreRBAC) Settings() store.SettingsStore                                { return nil }
func (m *mockStoreRBAC) Domains() store.DomainStore                                   { return nil }
func (m *mockStoreRBAC) Invitations() store.InvitationStore                           { return nil }
func (m *mockStoreRBAC) WithTx(ctx context.Context, fn func(store.Store) error) error { return nil }
func (m *mockStoreRBAC) Close() error                                                 { return nil }
func (m *mockStoreRBAC) Ping(ctx context.Context) error                               { return nil }

// genRBACUserID generates a valid user ID.
func genRBACUserID() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		if len(s) == 0 {
			return "user1"
		}
		if len(s) > 36 {
			return s[:36]
		}
		return s
	})
}

// genRBACEmail generates a valid email-like string.
func genRBACEmail() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier(),
		gen.Identifier(),
	).Map(func(vals []interface{}) string {
		local := vals[0].(string)
		domain := vals[1].(string)
		if len(local) == 0 {
			local = "user"
		}
		if len(domain) == 0 {
			domain = "example"
		}
		return local + "@" + domain + ".com"
	})
}

// genRole generates a valid role.
func genRole() gopter.Gen {
	return gen.OneConstOf(store.RoleOwner, store.RoleMember)
}

// genRBACUser generates a random user.
func genRBACUser() gopter.Gen {
	return gopter.CombineGens(
		genRBACUserID(),
		genRBACEmail(),
		genRole(),
	).Map(func(vals []interface{}) *store.User {
		return &store.User{
			ID:    vals[0].(string),
			Email: vals[1].(string),
			Role:  vals[2].(store.Role),
		}
	})
}

// genUsersWithOwner generates a list of users that includes at least one owner.
func genUsersWithOwner() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier().Map(func(s string) string {
			if len(s) == 0 {
				return "owner1"
			}
			return s
		}),
		gen.Identifier().Map(func(s string) string {
			if len(s) == 0 {
				return "owner"
			}
			return s + "@example.com"
		}),
		gen.IntRange(0, 5), // Number of additional users
	).Map(func(vals []interface{}) []*store.User {
		ownerID := vals[0].(string)
		ownerEmail := vals[1].(string)
		numOthers := vals[2].(int)

		// Create an owner user
		owner := &store.User{
			ID:    ownerID,
			Email: ownerEmail,
			Role:  store.RoleOwner,
		}

		// Create list with owner
		users := make([]*store.User, 0, numOthers+1)
		users = append(users, owner)

		// Add some member users
		for i := 0; i < numOthers; i++ {
			users = append(users, &store.User{
				ID:    ownerID + "_member_" + string(rune('a'+i)),
				Email: "member" + string(rune('a'+i)) + "@example.com",
				Role:  store.RoleMember,
			})
		}
		return users
	})
}

// genUsersWithoutOwner generates a list of users with no owners.
func genUsersWithoutOwner() gopter.Gen {
	return gen.IntRange(0, 5).Map(func(numUsers int) []*store.User {
		users := make([]*store.User, 0, numUsers)
		for i := 0; i < numUsers; i++ {
			users = append(users, &store.User{
				ID:    "member_" + string(rune('a'+i)),
				Email: "member" + string(rune('a'+i)) + "@example.com",
				Role:  store.RoleMember,
			})
		}
		return users
	})
}

func TestOwnerRegistrationBlocksPublicSignup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("When owner exists, public registration is blocked", prop.ForAll(
		func(users []*store.User) bool {
			// Create mock store with users
			mockUsers := &mockUserStoreRBAC{users: users}
			mockSt := &mockStoreRBAC{userStore: mockUsers}
			rbac := NewRBACService(mockSt, nil)

			// Check if registration is allowed
			canRegister, err := rbac.CanRegister(nil)
			if err != nil {
				return false
			}

			// If there's an owner, registration should be blocked
			hasOwner := false
			for _, u := range users {
				if u.Role == store.RoleOwner {
					hasOwner = true
					break
				}
			}

			// Property: canRegister should be false when owner exists
			return canRegister == !hasOwner
		},
		genUsersWithOwner(),
	))

	properties.Property("When no owner exists, public registration is allowed", prop.ForAll(
		func(users []*store.User) bool {
			// Create mock store with users (no owners)
			mockUsers := &mockUserStoreRBAC{users: users}
			mockSt := &mockStoreRBAC{userStore: mockUsers}
			rbac := NewRBACService(mockSt, nil)

			// Check if registration is allowed
			canRegister, err := rbac.CanRegister(nil)
			if err != nil {
				return false
			}

			// Property: canRegister should be true when no owner exists
			return canRegister == true
		},
		genUsersWithoutOwner(),
	))

	properties.TestingRun(t)
}

// **Feature: platform-enhancements, Property 10: RBAC Permission Enforcement**
// For any user with "member" role attempting to access admin functions (user management,
// system settings), the request SHALL be rejected with a forbidden error.
// **Validates: Requirements 14.5**

// genAdminPermission generates admin-only permissions.
func genAdminPermission() gopter.Gen {
	return gen.OneConstOf(
		PermissionManageUsers,
		PermissionManageSettings,
		PermissionViewUsers,
	)
}

// genMemberPermission generates permissions that members have.
func genMemberPermission() gopter.Gen {
	return gen.OneConstOf(
		PermissionViewApps,
		PermissionDeploy,
	)
}

// genAnyPermission generates any permission.
func genAnyPermission() gopter.Gen {
	return gen.OneConstOf(
		PermissionManageUsers,
		PermissionManageSettings,
		PermissionViewUsers,
		PermissionManageApps,
		PermissionViewApps,
		PermissionDeploy,
	)
}

func TestRBACPermissionEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("Members are denied admin permissions", prop.ForAll(
		func(permission Permission) bool {
			// Check that member role does not have admin permissions
			err := CheckRolePermission(store.RoleMember, permission)
			// Should return ErrPermissionDenied for admin permissions
			return err == ErrPermissionDenied
		},
		genAdminPermission(),
	))

	properties.Property("Members have basic permissions", prop.ForAll(
		func(permission Permission) bool {
			// Check that member role has basic permissions
			err := CheckRolePermission(store.RoleMember, permission)
			// Should return nil for member permissions
			return err == nil
		},
		genMemberPermission(),
	))

	properties.Property("Owners have all permissions", prop.ForAll(
		func(permission Permission) bool {
			// Check that owner role has all permissions
			err := CheckRolePermission(store.RoleOwner, permission)
			// Should return nil for all permissions
			return err == nil
		},
		genAnyPermission(),
	))

	properties.Property("Invalid roles are denied all permissions", prop.ForAll(
		func(permission Permission) bool {
			// Check that invalid role is denied all permissions
			err := CheckRolePermission(store.Role("invalid"), permission)
			// Should return ErrPermissionDenied
			return err == ErrPermissionDenied
		},
		genAnyPermission(),
	))

	properties.TestingRun(t)
}

func TestRBACServicePermissionCheck(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("RBACService correctly enforces permissions for members", prop.ForAll(
		func(userID string, permission Permission) bool {
			// Create a member user
			memberUser := &store.User{
				ID:    userID,
				Email: userID + "@example.com",
				Role:  store.RoleMember,
			}

			// Create mock store with the member user
			mockUsers := &mockUserStoreRBAC{users: []*store.User{memberUser}}
			mockSt := &mockStoreRBAC{userStore: mockUsers}
			rbac := NewRBACService(mockSt, nil)

			// Check permission
			err := rbac.CheckPermission(nil, userID, permission)

			// Expected result based on role permissions
			expectedErr := CheckRolePermission(store.RoleMember, permission)

			// Both should match
			if expectedErr == nil {
				return err == nil
			}
			return err != nil
		},
		gen.Identifier().Map(func(s string) string {
			if len(s) == 0 {
				return "user1"
			}
			return s
		}),
		genAnyPermission(),
	))

	properties.Property("RBACService correctly enforces permissions for owners", prop.ForAll(
		func(userID string, permission Permission) bool {
			// Create an owner user
			ownerUser := &store.User{
				ID:    userID,
				Email: userID + "@example.com",
				Role:  store.RoleOwner,
			}

			// Create mock store with the owner user
			mockUsers := &mockUserStoreRBAC{users: []*store.User{ownerUser}}
			mockSt := &mockStoreRBAC{userStore: mockUsers}
			rbac := NewRBACService(mockSt, nil)

			// Check permission
			err := rbac.CheckPermission(nil, userID, permission)

			// Owners should have all permissions
			return err == nil
		},
		gen.Identifier().Map(func(s string) string {
			if len(s) == 0 {
				return "owner1"
			}
			return s
		}),
		genAnyPermission(),
	))

	properties.TestingRun(t)
}
