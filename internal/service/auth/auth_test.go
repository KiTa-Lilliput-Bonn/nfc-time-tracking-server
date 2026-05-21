package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	svc := New("test-secret", 8)
	hash, err := svc.HashPassword("mypassword")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !svc.CheckPassword("mypassword", hash) {
		t.Error("expected password to match")
	}
	if svc.CheckPassword("wrongpassword", hash) {
		t.Error("expected wrong password to fail")
	}
}

func TestJWTIssueAndVerify(t *testing.T) {
	svc := New("test-jwt-secret", 8)
	token, err := svc.IssueToken(42, "admin", "superadmin")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	claims, err := svc.VerifyToken(token)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("expected user ID 42, got %d", claims.UserID)
	}
	if claims.Role != "superadmin" {
		t.Errorf("expected role superadmin, got %s", claims.Role)
	}
}

func TestJWTInvalidSecret(t *testing.T) {
	svc1 := New("secret-1", 8)
	svc2 := New("secret-2", 8)
	token, _ := svc1.IssueToken(1, "user", "user")
	_, err := svc2.VerifyToken(token)
	if err == nil {
		t.Error("expected verification to fail with wrong secret")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	p := GenerateRandomPassword(16)
	if len(p) != 16 {
		t.Errorf("expected length 16, got %d", len(p))
	}
}
