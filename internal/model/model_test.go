package model

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTargetService_Validate(t *testing.T) {
	tests := []struct {
		name    string
		target  TargetService
		wantErr bool
	}{
		{
			name: "Valid TCP",
			target: TargetService{
				Protocol: ProtocolTCP,
				Host:     "10.0.0.1",
				Port:     5432,
			},
			wantErr: false,
		},
		{
			name: "Invalid TCP port 0",
			target: TargetService{
				Protocol: ProtocolTCP,
				Host:     "db.corp.local",
				Port:     0,
			},
			wantErr: true,
		},
		{
			name: "Valid HTTP without port defaults to 80",
			target: TargetService{
				Protocol: ProtocolHTTP,
				Host:     "api.corp.local",
			},
			wantErr: false,
		},
		{
			name: "Valid HTTPS without port defaults to 443",
			target: TargetService{
				Protocol: ProtocolHTTPS,
				Host:     "auth.corp.local",
			},
			wantErr: false,
		},
		{
			name: "Valid ICMP",
			target: TargetService{
				Protocol: ProtocolICMP,
				Host:     "8.8.8.8",
			},
			wantErr: false,
		},
		{
			name: "Valid TCP with stale path and tls - cleans them",
			target: TargetService{
				Protocol:    ProtocolTCP,
				Host:        "10.0.0.1",
				Port:        5432,
				Path:        "/should-be-cleared",
				InsecureTLS: true,
			},
			wantErr: false,
		},
		{
			name: "Valid ICMP with stale port, path and tls - cleans them",
			target: TargetService{
				Protocol:    ProtocolICMP,
				Host:        "8.8.8.8",
				Port:        8080,
				Path:        "/ping",
				InsecureTLS: true,
			},
			wantErr: false,
		},
		{
			name: "Valid HTTP with stale tls - cleans tls",
			target: TargetService{
				Protocol:    ProtocolHTTP,
				Host:        "api.local",
				Port:        80,
				InsecureTLS: true,
			},
			wantErr: false,
		},
		{
			name: "Empty Host",
			target: TargetService{
				Protocol: ProtocolTCP,
				Host:     "",
				Port:     80,
			},
			wantErr: true,
		},
		{
			name: "Unsupported Protocol",
			target: TargetService{
				Protocol: "ftp",
				Host:     "files.corp.local",
				Port:     21,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.target.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.target.Protocol == ProtocolHTTP && tt.target.Port != 80 && tt.target.Port == 0 {
				t.Errorf("Expected HTTP default port 80, got %d", tt.target.Port)
			}
			if !tt.wantErr && (tt.target.Protocol == ProtocolTCP || tt.target.Protocol == ProtocolUDP || tt.target.Protocol == ProtocolICMP) && tt.target.Path != "" {
				t.Errorf("Expected empty Path for %s, got %q", tt.target.Protocol, tt.target.Path)
			}
			if !tt.wantErr && tt.target.Protocol == ProtocolICMP && tt.target.Port != 0 {
				t.Errorf("Expected Port 0 for ICMP, got %d", tt.target.Port)
			}
			if !tt.wantErr && (tt.target.Protocol == ProtocolTCP || tt.target.Protocol == ProtocolUDP || tt.target.Protocol == ProtocolHTTP || tt.target.Protocol == ProtocolICMP) && tt.target.InsecureTLS {
				t.Errorf("Expected InsecureTLS false for %s, got true", tt.target.Protocol)
			}
		})
	}
}

func TestCheckRequest_Validate(t *testing.T) {
	req := CheckRequest{
		Mode: ModeHostNetwork,
		Targets: []TargetService{
			{Protocol: ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}
	if err := req.Validate(false, "kncc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test multi-namespace check when disabled
	reqNamespace := CheckRequest{
		Mode:      ModePodNetwork,
		Namespace: "custom-app",
		Targets: []TargetService{
			{Protocol: ProtocolTCP, Host: "10.0.0.1", Port: 80},
		},
	}
	if err := reqNamespace.Validate(false, "kncc"); err == nil {
		t.Fatalf("expected error when multi-namespace is disabled, got nil")
	}

	// Test multi-namespace check when enabled
	if err := reqNamespace.Validate(true, "kncc"); err != nil {
		t.Fatalf("unexpected error when multi-namespace is enabled: %v", err)
	}
}

func TestProbeOutput_CertFields(t *testing.T) {
	days := 45
	probe := ProbeOutput{
		Target: TargetService{
			Protocol: ProtocolHTTPS,
			Host:     "secure.example.com",
			Port:     443,
		},
		Success:      true,
		LatencyMs:    25,
		CertExpiry:   "2027-11-03 15:25:23Z",
		CertDaysLeft: &days,
		CertStatus:   CertStatusValid,
		CertSubject:  "CN = secure.example.com",
		CertIssuer:   "CN = Example CA",
	}

	if probe.CertStatus != CertStatusValid {
		t.Errorf("expected CertStatusValid, got %s", probe.CertStatus)
	}
	if *probe.CertDaysLeft != 45 {
		t.Errorf("expected CertDaysLeft 45, got %d", *probe.CertDaysLeft)
	}
}

func TestTargetService_UnmarshalYAML_SnakeAndCamelCase(t *testing.T) {
	yamlInput := `
- name: "Postgres TCP"
  protocol: "tcp"
  host: "postgres.default.svc.cluster.local"
  port: 5432
  timeout_seconds: 7
- name: "CoreDNS UDP"
  protocol: "udp"
  host: "kube-dns.kube-system.svc.cluster.local"
  port: 53
  timeoutSeconds: 4
- name: "App HTTP"
  protocol: "http"
  host: "frontend.kncc.svc.cluster.local"
  port: 8080
  path: "/healthz"
  timeout_seconds: 5
- name: "API HTTPS CamelCase"
  protocol: "https"
  host: "kubernetes.default.svc.cluster.local"
  port: 443
  path: "/livez"
  insecureTls: true
  timeoutSeconds: 10
- name: "API HTTPS SnakeCase"
  protocol: "https"
  host: "kubernetes.default.svc.cluster.local"
  port: 6443
  path: "/readyz"
  insecure_tls: true
  timeout_seconds: 8
- name: "Gateway ICMP"
  protocol: "icmp"
  host: "10.0.0.1"
  timeout_seconds: 3
`
	var targets []TargetService
	if err := yaml.Unmarshal([]byte(yamlInput), &targets); err != nil {
		t.Fatalf("failed to unmarshal TargetService YAML: %v", err)
	}

	if len(targets) != 6 {
		t.Fatalf("expected 6 targets, got %d", len(targets))
	}

	// 1. TCP
	if targets[0].Protocol != ProtocolTCP || targets[0].Port != 5432 || targets[0].TimeoutSeconds != 7 {
		t.Errorf("unexpected TCP target: %+v", targets[0])
	}
	// 2. UDP with camelCase timeoutSeconds
	if targets[1].Protocol != ProtocolUDP || targets[1].Port != 53 || targets[1].TimeoutSeconds != 4 {
		t.Errorf("unexpected UDP target: %+v", targets[1])
	}
	// 3. HTTP with path
	if targets[2].Protocol != ProtocolHTTP || targets[2].Path != "/healthz" || targets[2].Port != 8080 {
		t.Errorf("unexpected HTTP target: %+v", targets[2])
	}
	// 4. HTTPS with camelCase insecureTls & timeoutSeconds
	if targets[3].Protocol != ProtocolHTTPS || !targets[3].InsecureTLS || targets[3].TimeoutSeconds != 10 {
		t.Errorf("unexpected HTTPS target (camelCase): %+v", targets[3])
	}
	// 5. HTTPS with snake_case insecure_tls & timeout_seconds
	if targets[4].Protocol != ProtocolHTTPS || !targets[4].InsecureTLS || targets[4].TimeoutSeconds != 8 {
		t.Errorf("unexpected HTTPS target (snake_case): %+v", targets[4])
	}
	// 6. ICMP
	if targets[5].Protocol != ProtocolICMP || targets[5].Host != "10.0.0.1" || targets[5].TimeoutSeconds != 3 {
		t.Errorf("unexpected ICMP target: %+v", targets[5])
	}

	// Validate all
	for i, trg := range targets {
		if err := trg.Validate(); err != nil {
			t.Errorf("target %d (%s) failed validation: %v", i, trg.Name, err)
		}
	}
}
