package k8s

import (
	"strings"
	"testing"

	"kncc/internal/model"
)

func TestBuildProbeScript(t *testing.T) {
	targets := []model.TargetService{
		{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 5432, TimeoutSeconds: 3},
		{Protocol: model.ProtocolHTTP, Host: "api.corp.local", Port: 80, Path: "/healthz", TimeoutSeconds: 5},
		{Protocol: model.ProtocolHTTPS, Host: "secure.corp.local", Port: 443, Path: "/", InsecureTLS: true, TimeoutSeconds: 5},
		{Protocol: model.ProtocolUDP, Host: "dns.corp.local", Port: 53, TimeoutSeconds: 2},
		{Protocol: model.ProtocolICMP, Host: "8.8.8.8", TimeoutSeconds: 2},
	}

	script, err := BuildProbeScript(targets)
	if err != nil {
		t.Fatalf("BuildProbeScript error: %v", err)
	}

	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Errorf("script must start with #!/bin/sh")
	}

	// Verify protocol sections are present
	expectedSnippets := []string{
		"nc -z -w \"$TIMEOUT\" \"$HOST\" \"$PORT\"",
		"curl -s -S -o /dev/null",
		"INSECURE_FLAG=\"-k\"",
		"nc -u -z -w \"$TIMEOUT\" \"$HOST\" \"$PORT\"",
		"ping -c 2 -W \"$TIMEOUT\" \"$HOST\"",
		"LOG_FILE=\"/dev/termination-log\"",
		"exit 0",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Errorf("expected script to contain snippet %q", snippet)
		}
	}

	// Verify HTTPS certificate extraction is included
	httpsSnippets := []string{
		"openssl s_client -connect \"$HOST:$PORT\" -servername \"$HOST\"",
		"openssl x509 -dateopt iso_8601 -noout -enddate",
		"\"cert_expiry\": \"$CERT_EXP_CLEAN\"",
		"\"cert_subject\": \"$CERT_SUBJ_CLEAN\"",
		"\"cert_issuer\": \"$CERT_ISS_CLEAN\"",
	}
	for _, snippet := range httpsSnippets {
		if !strings.Contains(script, snippet) {
			t.Errorf("expected HTTPS script to contain snippet %q", snippet)
		}
	}

	// Verify that targets without HTTPS do NOT contain openssl s_client
	tcpOnly := []model.TargetService{
		{Protocol: model.ProtocolTCP, Host: "10.0.0.1", Port: 80, TimeoutSeconds: 3},
	}
	tcpScript, err := BuildProbeScript(tcpOnly)
	if err != nil {
		t.Fatalf("BuildProbeScript for TCP failed: %v", err)
	}
	if strings.Contains(tcpScript, "openssl s_client") {
		t.Errorf("TCP-only probe should not include openssl s_client check")
	}
}

func TestBuildProbeScriptWithCA(t *testing.T) {
	targets := []model.TargetService{
		{Protocol: model.ProtocolHTTPS, Host: "internal-api.corp.local", Port: 8443, Path: "/health", InsecureTLS: false, TimeoutSeconds: 5},
	}

	customCA := "-----BEGIN CERTIFICATE-----\nMIIB...fake...corp...ca\n-----END CERTIFICATE-----"
	script, err := BuildProbeScriptWithCA(targets, customCA)
	if err != nil {
		t.Fatalf("BuildProbeScriptWithCA failed: %v", err)
	}

	expectedSnippets := []string{
		"cat << 'EOF_KNCC_CA' > /tmp/kncc-certs/custom-ca.pem",
		"MIIB...fake...corp...ca",
		"CA_BUNDLE=\"/tmp/kncc-certs/ca-bundle.crt\"",
		"export SSL_CERT_FILE=\"$CA_BUNDLE\"",
		"export CURL_CA_BUNDLE=\"$CA_BUNDLE\"",
		"CA_FLAG=\"--cacert $CA_BUNDLE\"",
		"OPENSSL_CA_FLAG=\"-CAfile $CA_BUNDLE\"",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Errorf("expected script to contain %q, but it was missing", snippet)
		}
	}
}

func TestBuildProbeScriptIPResolutionOptimization(t *testing.T) {
	// IP target should bypass getent hosts
	ipTarget := []model.TargetService{
		{Protocol: model.ProtocolTCP, Host: "10.96.0.10", Port: 80, TimeoutSeconds: 3},
		{Protocol: model.ProtocolICMP, Host: "8.8.8.8", TimeoutSeconds: 2},
	}
	ipScript, err := BuildProbeScript(ipTarget)
	if err != nil {
		t.Fatalf("BuildProbeScript for IP target failed: %v", err)
	}
	if strings.Contains(ipScript, "getent hosts") {
		t.Errorf("IP target script should NOT contain getent hosts lookup")
	}
	if !strings.Contains(ipScript, "RESOLVED_IP=\"$HOST\"") {
		t.Errorf("IP target script must directly assign RESOLVED_IP=\"$HOST\"")
	}

	// Hostname target should include getent hosts and nslookup fallback
	hostTarget := []model.TargetService{
		{Protocol: model.ProtocolHTTP, Host: "dex.local.example.com", Port: 80, Path: "/healthz", TimeoutSeconds: 3},
	}
	hostScript, err := BuildProbeScript(hostTarget)
	if err != nil {
		t.Fatalf("BuildProbeScript for hostname target failed: %v", err)
	}
	if !strings.Contains(hostScript, "getent hosts \"$HOST\"") {
		t.Errorf("Hostname target script must contain getent hosts lookup")
	}
	if !strings.Contains(hostScript, "nslookup \"$HOST\"") {
		t.Errorf("Hostname target script must contain nslookup fallback")
	}
}
