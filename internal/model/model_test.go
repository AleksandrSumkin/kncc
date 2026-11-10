package model

import (
	"testing"
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
