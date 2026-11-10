package security

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"kncc/internal/config"
	"kncc/internal/model"
)

// TargetValidator checks target services against security policies (Blacklists, Whitelists, Anti-SSRF).
type TargetValidator struct {
	blacklistCIDRs   []*net.IPNet
	blacklistDomains []string
	whitelistCIDRs   []*net.IPNet
	whitelistDomains []string
	lookupIPFn       func(ctx context.Context, host string) ([]net.IP, error)
}

// NewTargetValidator creates a new validator from security configuration.
func NewTargetValidator(cfg *config.SecurityConfig) (*TargetValidator, error) {
	v := &TargetValidator{
		blacklistDomains: cfg.BlacklistDomains,
		whitelistDomains: cfg.WhitelistDomains,
		lookupIPFn: func(ctx context.Context, host string) ([]net.IP, error) {
			r := net.DefaultResolver
			return r.LookupIP(ctx, "ip", host)
		},
	}

	// Parse blacklist CIDRs
	for _, cidr := range cfg.BlacklistCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			// If plain IP, treat as /32 or /128
			if strings.Contains(cidr, ":") {
				cidr += "/128"
			} else {
				cidr += "/32"
			}
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid blacklist CIDR %q: %w", cidr, err)
		}
		v.blacklistCIDRs = append(v.blacklistCIDRs, ipNet)
	}

	// Parse whitelist CIDRs
	for _, cidr := range cfg.WhitelistCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			if strings.Contains(cidr, ":") {
				cidr += "/128"
			} else {
				cidr += "/32"
			}
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid whitelist CIDR %q: %w", cidr, err)
		}
		v.whitelistCIDRs = append(v.whitelistCIDRs, ipNet)
	}

	return v, nil
}

// ValidateTarget verifies whether a target is permitted to be checked.
func (v *TargetValidator) ValidateTarget(ctx context.Context, target model.TargetService) error {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return fmt.Errorf("target host cannot be empty")
	}

	lowerHost := strings.ToLower(host)

	// 1. Check Blacklist Domains
	for _, bd := range v.blacklistDomains {
		bd = strings.ToLower(strings.TrimSpace(bd))
		if bd == "" {
			continue
		}
		if lowerHost == bd || strings.HasSuffix(lowerHost, "."+bd) {
			return fmt.Errorf("target host %q is prohibited by security policy (domain blacklist)", host)
		}
	}

	// 2. Check if host is direct IP
	parsedIP := net.ParseIP(host)
	if parsedIP != nil {
		return v.validateIP(parsedIP)
	}

	// 3. Check Whitelist Domains (if configured)
	if len(v.whitelistDomains) > 0 {
		matched := false
		for _, wd := range v.whitelistDomains {
			wd = strings.ToLower(strings.TrimSpace(wd))
			if wd == "" {
				continue
			}
			if lowerHost == wd || strings.HasSuffix(lowerHost, "."+wd) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("target domain %q is not in the allowed domains whitelist", host)
		}
	}

	// 4. Resolve domain and check resolved IP against Blacklist CIDRs (anti-SSRF / DNS rebinding protection)
	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	ips, err := v.lookupIPFn(resolveCtx, host)
	if err == nil && len(ips) > 0 {
		for _, ip := range ips {
			if err := v.validateIP(ip); err != nil {
				return fmt.Errorf("target %q resolves to %s which is prohibited: %w", host, ip.String(), err)
			}
		}
	}

	return nil
}

func (v *TargetValidator) validateIP(ip net.IP) error {
	// Check against Blacklist CIDRs
	for _, bCIDR := range v.blacklistCIDRs {
		if bCIDR.Contains(ip) {
			return fmt.Errorf("IP %s matches prohibited CIDR %s", ip.String(), bCIDR.String())
		}
	}

	// Check against Whitelist CIDRs (if configured)
	if len(v.whitelistCIDRs) > 0 {
		matched := false
		for _, wCIDR := range v.whitelistCIDRs {
			if wCIDR.Contains(ip) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("IP %s is not within allowed whitelist CIDRs", ip.String())
		}
	}

	return nil
}
