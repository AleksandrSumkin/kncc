package security

import (
	"context"
	"net"
	"testing"

	"kncc/internal/config"
	"kncc/internal/model"
)

func TestTargetValidator_Blacklist(t *testing.T) {
	cfg := &config.SecurityConfig{
		BlacklistCIDRs: []string{
			"169.254.169.254/32",
			"127.0.0.0/8",
			"10.96.0.0/12",
		},
		BlacklistDomains: []string{
			"metadata.google.internal",
			"bad.corp",
		},
	}

	v, err := NewTargetValidator(cfg)
	if err != nil {
		t.Fatalf("NewTargetValidator error: %v", err)
	}

	ctx := context.Background()

	// 1. Block 169.254.169.254 directly
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolTCP, Host: "169.254.169.254", Port: 80})
	if err == nil {
		t.Errorf("expected error for cloud metadata IP 169.254.169.254, got nil")
	}

	// 2. Block 127.0.0.1 loopback
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolTCP, Host: "127.0.0.1", Port: 80})
	if err == nil {
		t.Errorf("expected error for loopback 127.0.0.1, got nil")
	}

	// 3. Block blacklist domain
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolHTTP, Host: "metadata.google.internal", Port: 80})
	if err == nil {
		t.Errorf("expected error for metadata.google.internal, got nil")
	}

	// 4. Block subdomain of blacklist domain
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolHTTP, Host: "sub.bad.corp", Port: 80})
	if err == nil {
		t.Errorf("expected error for sub.bad.corp, got nil")
	}

	// 5. Allow benign public host
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolTCP, Host: "8.8.8.8", Port: 53})
	if err != nil {
		t.Errorf("unexpected error for 8.8.8.8: %v", err)
	}
}

func TestTargetValidator_DNSResolutionBlacklist(t *testing.T) {
	cfg := &config.SecurityConfig{
		BlacklistCIDRs: []string{"169.254.169.254/32"},
	}

	v, err := NewTargetValidator(cfg)
	if err != nil {
		t.Fatalf("NewTargetValidator error: %v", err)
	}

	// Mock DNS lookup to return 169.254.169.254 for evil.com
	v.lookupIPFn = func(ctx context.Context, host string) ([]net.IP, error) {
		if host == "evil.com" {
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		}
		return []net.IP{net.ParseIP("1.1.1.1")}, nil
	}

	ctx := context.Background()

	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolHTTP, Host: "evil.com", Port: 80})
	if err == nil {
		t.Errorf("expected error for evil.com resolving to 169.254.169.254, got nil")
	}

	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolHTTP, Host: "good.com", Port: 80})
	if err != nil {
		t.Errorf("unexpected error for good.com: %v", err)
	}
}

func TestTargetValidator_Whitelist(t *testing.T) {
	cfg := &config.SecurityConfig{
		WhitelistCIDRs:   []string{"10.0.0.0/8"},
		WhitelistDomains: []string{"allowed.corp"},
	}

	v, err := NewTargetValidator(cfg)
	if err != nil {
		t.Fatalf("NewTargetValidator error: %v", err)
	}

	ctx := context.Background()

	// Allowed by CIDR whitelist
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolTCP, Host: "10.1.2.3", Port: 80})
	if err != nil {
		t.Errorf("unexpected error for 10.1.2.3: %v", err)
	}

	// Prohibited outside CIDR whitelist
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolTCP, Host: "192.168.1.1", Port: 80})
	if err == nil {
		t.Errorf("expected error for 192.168.1.1 not in whitelist, got nil")
	}

	// Allowed by domain whitelist
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolHTTP, Host: "api.allowed.corp", Port: 80})
	if err != nil {
		t.Errorf("unexpected error for api.allowed.corp: %v", err)
	}

	// Prohibited outside domain whitelist
	err = v.ValidateTarget(ctx, model.TargetService{Protocol: model.ProtocolHTTP, Host: "other.corp", Port: 80})
	if err == nil {
		t.Errorf("expected error for other.corp not in whitelist, got nil")
	}
}
