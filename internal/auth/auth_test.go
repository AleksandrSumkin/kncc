package auth

import (
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"
	"kncc/internal/store"

	"github.com/go-ldap/ldap/v3"
	"golang.org/x/crypto/bcrypt"
)

func TestJWTManager(t *testing.T) {
	jwtMgr := NewJWTManager("super-secret-key-1234567890", 1*time.Hour)

	user := &model.User{
		ID:       "u-123",
		Username: "alice",
		Role:     model.RoleAdmin,
		Provider: "local",
	}

	token, err := jwtMgr.GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := jwtMgr.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != "u-123" || claims.Username != "alice" || claims.Role != model.RoleAdmin {
		t.Errorf("claims mismatch: %+v", claims)
	}

	// Validate with different key must fail
	otherMgr := NewJWTManager("different-secret-key-0987654321", 1*time.Hour)
	_, err = otherMgr.ValidateToken(token)
	if err == nil {
		t.Errorf("expected validation to fail with different secret, got nil")
	}
}

func TestAuthService_LocalAuth(t *testing.T) {
	dir, err := os.MkdirTemp("", "kncc-auth-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "auth_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create test user
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	testUser := &model.User{
		ID:           "u-1",
		Username:     "bob",
		PasswordHash: string(hash),
		Role:         model.RoleUser,
		Provider:     "local",
	}
	if err := s.CreateUser(ctx, testUser); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	cfg := &config.AuthConfig{
		LocalEnabled: true,
	}
	jwtMgr := NewJWTManager("secret-key", 2*time.Hour)

	authSvc, err := NewAuthService(ctx, cfg, s, jwtMgr)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}

	// Correct password
	u, token, err := authSvc.AuthenticateLocal(ctx, "bob", "correct-password")
	if err != nil {
		t.Fatalf("AuthenticateLocal with correct password failed: %v", err)
	}
	if u.Username != "bob" || token == "" {
		t.Errorf("invalid user or empty token")
	}

	// Wrong password
	_, _, err = authSvc.AuthenticateLocal(ctx, "bob", "wrong-password")
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for wrong password, got: %v", err)
	}

	// Non-existent user
	_, _, err = authSvc.AuthenticateLocal(ctx, "unknown-user", "pass")
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for unknown user, got: %v", err)
	}
}

func TestAuthService_WithRootCAs(t *testing.T) {
	ctx := context.Background()
	pool := x509.NewCertPool()
	cfg := &config.AuthConfig{
		LocalEnabled: true,
	}
	jwtMgr := NewJWTManager("secret-key", 2*time.Hour)

	authSvc, err := NewAuthService(ctx, cfg, nil, jwtMgr, pool)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}
	if authSvc.rootCAs != pool {
		t.Errorf("expected custom rootCAs pool to be set")
	}
	if authSvc.httpClient == nil {
		t.Errorf("expected httpClient to be configured with custom rootCAs")
	}
}

func TestParseCN(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"CN=RBAC_KNCC_ADMIN,OU=Groups,DC=example,DC=com", "RBAC_KNCC_ADMIN"},
		{"cn=developers,ou=groups,dc=corp,dc=local", "developers"},
		{"PlainGroupName", "PlainGroupName"},
		{"CN=With Space,OU=Users", "With Space"},
	}

	for _, c := range cases {
		got := parseCN(c.input)
		if got != c.expected {
			t.Errorf("parseCN(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestGroupFilterMatching(t *testing.T) {
	cases := []struct {
		groupName string
		filter    string
		expected  bool
	}{
		{"RBAC_KNCC_ADMIN", "(cn=RBAC_KNCC_*)", true},
		{"RBAC_KNCC_OPERATOR", "(cn=RBAC_KNCC_*)", true},
		{"DOMAIN_USERS", "(cn=RBAC_KNCC_*)", false},
		{"OTHER_GROUP", "RBAC_KNCC_*", false},
		{"RBAC_KNCC_GUEST", "RBAC_KNCC_*", true},
		{"RBAC_KNCC_ADMIN", "(&(objectClass=group)(cn=RBAC_KNCC_*))", true},
		{"ANY_GROUP", "", true},
		{"kncc-admin", "kncc-admin", true},
		{"kncc-user", "kncc-admin", false},
	}

	for _, c := range cases {
		got := matchesGroupFilter(c.groupName, c.filter)
		if got != c.expected {
			t.Errorf("matchesGroupFilter(%q, %q) = %v, expected %v", c.groupName, c.filter, got, c.expected)
		}
	}
}

func TestAuthService_FetchUserLDAPGroupsAndFilter(t *testing.T) {
	cfg := &config.AuthConfig{
		LDAPEnabled: true,
		LDAP: config.LDAPConfig{
			AdminGroup:  "RBAC_KNCC_ADMIN",
			GroupFilter: "(cn=RBAC_KNCC_*)",
		},
	}

	authSvc, err := NewAuthService(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}

	// Create mock LDAP Entry with memberOf attributes
	entry := &ldap.Entry{
		DN: "uid=alice,ou=users,dc=example,dc=com",
		Attributes: []*ldap.EntryAttribute{
			{
				Name: "memberOf",
				Values: []string{
					"CN=RBAC_KNCC_ADMIN,OU=Groups,DC=example,DC=com",
					"CN=RBAC_KNCC_DEVELOPER,OU=Groups,DC=example,DC=com",
					"CN=Domain Users,CN=Users,DC=example,DC=com",
					"CN=VPN_Access,OU=Groups,DC=example,DC=com",
				},
			},
		},
	}

	// fetchUserLDAPGroups with nil conn tests memberOf extraction + groupFilter matching
	groups, err := authSvc.fetchUserLDAPGroups(nil, entry.DN, "alice", entry)
	if err != nil {
		t.Fatalf("fetchUserLDAPGroups failed: %v", err)
	}

	// Check that only groups matching RBAC_KNCC_* are returned
	hasAdmin := false
	hasDeveloper := false
	hasDomainUsers := false
	for _, g := range groups {
		cn := parseCN(g)
		if cn == "RBAC_KNCC_ADMIN" {
			hasAdmin = true
		}
		if cn == "RBAC_KNCC_DEVELOPER" {
			hasDeveloper = true
		}
		if cn == "Domain Users" || cn == "VPN_Access" {
			hasDomainUsers = true
		}
	}

	if !hasAdmin {
		t.Errorf("expected RBAC_KNCC_ADMIN in filtered groups: %v", groups)
	}
	if !hasDeveloper {
		t.Errorf("expected RBAC_KNCC_DEVELOPER in filtered groups: %v", groups)
	}
	if hasDomainUsers {
		t.Errorf("unwanted non-application groups leaked through groupFilter: %v", groups)
	}
}

func TestAuthService_DynamicRoleRecalculation(t *testing.T) {
	dir, err := os.MkdirTemp("", "kncc-ldap-role-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "role_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Initial user in SQLite with role 'user'
	initialUser := &model.User{
		ID:        "ldap-user-42",
		Username:  "carol",
		Role:      model.RoleUser,
		Provider:  "ldap",
		Email:     "carol@corp.local",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.CreateUser(ctx, initialUser); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	cfg := &config.AuthConfig{
		AdminUsername: "superadmin",
		LDAPEnabled:   true,
		LDAP: config.LDAPConfig{
			AdminGroup:  "RBAC_KNCC_ADMIN",
			GroupFilter: "(cn=RBAC_KNCC_*)",
		},
	}
	jwtMgr := NewJWTManager("secret-key", 2*time.Hour)
	authSvc, err := NewAuthService(ctx, cfg, s, jwtMgr)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}

	// 1. Simulate login with entry that HAS the admin group -> should be promoted to RoleAdmin
	entryWithAdmin := &ldap.Entry{
		DN: "uid=carol,ou=users,dc=example,dc=com",
		Attributes: []*ldap.EntryAttribute{
			{Name: "memberOf", Values: []string{"CN=RBAC_KNCC_ADMIN,OU=Groups,DC=example,DC=com"}},
		},
	}
	groups, _ := authSvc.fetchUserLDAPGroups(nil, entryWithAdmin.DN, "carol", entryWithAdmin)
	isAdmin := false
	for _, g := range groups {
		if parseCN(g) == cfg.LDAP.AdminGroup {
			isAdmin = true
			break
		}
	}
	if !isAdmin {
		t.Fatalf("expected user to be admin with admin group present")
	}

	// Update in store as AuthenticateLDAP does
	newRole := model.RoleAdmin
	if err := s.UpdateUserRole(ctx, initialUser.ID, newRole); err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}
	u1, _ := s.GetUserByID(ctx, initialUser.ID)
	if u1.Role != model.RoleAdmin {
		t.Errorf("expected user role promoted to admin in SQLite, got %s", u1.Role)
	}

	// 2. Simulate next login where user was REMOVED from the admin group in LDAP
	entryWithoutAdmin := &ldap.Entry{
		DN: "uid=carol,ou=users,dc=example,dc=com",
		Attributes: []*ldap.EntryAttribute{
			{Name: "memberOf", Values: []string{"CN=RBAC_KNCC_VIEWER,OU=Groups,DC=example,DC=com"}},
		},
	}
	groups2, _ := authSvc.fetchUserLDAPGroups(nil, entryWithoutAdmin.DN, "carol", entryWithoutAdmin)
	isAdmin2 := false
	for _, g := range groups2 {
		if parseCN(g) == cfg.LDAP.AdminGroup {
			isAdmin2 = true
			break
		}
	}
	if isAdmin2 {
		t.Fatalf("expected user NOT to be admin after group removal")
	}

	// Demote in store
	demotedRole := model.RoleUser
	if err := s.UpdateUserRole(ctx, initialUser.ID, demotedRole); err != nil {
		t.Fatalf("UpdateUserRole failed: %v", err)
	}
	u2, _ := s.GetUserByID(ctx, initialUser.ID)
	if u2.Role != model.RoleUser {
		t.Errorf("expected user role demoted back to user in SQLite, got %s", u2.Role)
	}
}

func TestAuthService_AuthenticateLDAP_Validation(t *testing.T) {
	ctx := context.Background()

	// Provider disabled
	cfgDisabled := &config.AuthConfig{LDAPEnabled: false}
	svcDisabled, err := NewAuthService(ctx, cfgDisabled, nil, nil)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}
	_, _, err = svcDisabled.AuthenticateLDAP(ctx, "user", "pass")
	if err != ErrProviderDisabled {
		t.Errorf("expected ErrProviderDisabled, got %v", err)
	}

	// Empty username or password
	cfgEnabled := &config.AuthConfig{LDAPEnabled: true}
	svcEnabled, err := NewAuthService(ctx, cfgEnabled, nil, nil)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}
	_, _, err = svcEnabled.AuthenticateLDAP(ctx, "", "pass")
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for empty username, got %v", err)
	}
	_, _, err = svcEnabled.AuthenticateLDAP(ctx, "user", "")
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for empty password, got %v", err)
	}
}

func TestAuthService_DirectBind_DNFormatting(t *testing.T) {
	cases := []struct {
		template string
		username string
		expected string
	}{
		{"uid=%s,ou=users,dc=kncc,dc=local", "alice", "uid=alice,ou=users,dc=kncc,dc=local"},
		{"cn=%s,ou=people,dc=example,dc=com", "john doe", "cn=john doe,ou=people,dc=example,dc=com"},
		{"%s@corp.domain", "admin", "admin@corp.domain"},
		{"uid=%s,ou=users,dc=example,dc=com", "special,user", "uid=special\\,user,ou=users,dc=example,dc=com"},
	}

	for _, c := range cases {
		got := c.template
		if strings.Contains(c.template, "%s") {
			got = strings.Replace(c.template, "%s", ldap.EscapeDN(c.username), 1)
		}
		if got != c.expected {
			t.Errorf("formatting (%q, %q) = %q, expected %q", c.template, c.username, got, c.expected)
		}
	}
}
