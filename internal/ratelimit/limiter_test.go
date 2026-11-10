package ratelimit

import (
	"testing"
)

func TestLimiter_Allow(t *testing.T) {
	limit := 3
	l := NewLimiter(limit)
	defer l.Stop()

	user := "user-alice"

	// First 3 requests must succeed
	for i := 0; i < limit; i++ {
		if !l.Allow(user) {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 4th request must be rejected
	if l.Allow(user) {
		t.Errorf("4th request should have been rejected by rate limiter")
	}

	// Different user should still be allowed
	if !l.Allow("user-bob") {
		t.Errorf("request from user-bob should have been allowed")
	}
}
