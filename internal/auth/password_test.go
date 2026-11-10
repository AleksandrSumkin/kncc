package auth

import (
	"strings"
	"testing"
)

func TestGenerateSecurePassword(t *testing.T) {
	// 1. Default length
	p1 := GenerateSecurePassword(0)
	if len(p1) != DefaultPasswordLength {
		t.Fatalf("expected default length %d, got %d", DefaultPasswordLength, len(p1))
	}

	// 2. Custom length
	p2 := GenerateSecurePassword(24)
	if len(p2) != 24 {
		t.Fatalf("expected custom length 24, got %d", len(p2))
	}

	// 3. Characters must be from charset
	for _, ch := range p2 {
		if !strings.ContainsRune(PasswordCharset, ch) {
			t.Fatalf("character %c not in allowed charset", ch)
		}
	}

	// 4. Randomness: consecutive calls must not produce the same password
	p3 := GenerateSecurePassword(16)
	p4 := GenerateSecurePassword(16)
	if p3 == p4 {
		t.Fatalf("consecutive generated passwords matched: %s", p3)
	}
}
