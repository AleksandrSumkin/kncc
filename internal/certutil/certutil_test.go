package certutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper to generate self-signed Root CA and child Intermediate CA
func generateTestCAChain(t *testing.T) (rootPEM, intermediatePEM, leafPEM []byte, leafCert *x509.Certificate) {
	t.Helper()

	// 1. Root CA
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate root key: %v", err)
	}

	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1001),
		Subject: pkix.Name{
			CommonName:   "Corp Root CA",
			Organization: []string{"Acme Corp"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("failed to create root certificate: %v", err)
	}
	rootPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})

	// 2. Intermediate CA
	intermediateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate intermediate key: %v", err)
	}

	intermediateTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1002),
		Subject: pkix.Name{
			CommonName:   "Corp Intermediate CA",
			Organization: []string{"Acme Corp"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	intermediateDER, err := x509.CreateCertificate(rand.Reader, intermediateTemplate, rootTemplate, &intermediateKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("failed to create intermediate certificate: %v", err)
	}
	intermediatePEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: intermediateDER})

	// 3. Leaf server certificate signed by Intermediate CA
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate leaf key: %v", err)
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1003),
		Subject: pkix.Name{
			CommonName:   "internal.corp.example.com",
			Organization: []string{"Acme Corp"},
		},
		DNSNames:    []string{"internal.corp.example.com"},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(12 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, intermediateTemplate, &leafKey.PublicKey, intermediateKey)
	if err != nil {
		t.Fatalf("failed to create leaf certificate: %v", err)
	}
	leafPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

	leafCert, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("failed to parse leaf certificate: %v", err)
	}

	return rootPEM, intermediatePEM, leafPEM, leafCert
}

func TestLoadCertPool_Empty(t *testing.T) {
	pool, bundle, err := LoadCertPool("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	if bundle != "" {
		t.Fatalf("expected empty bundle, got: %s", bundle)
	}
}

func TestLoadCertPool_InlineChain(t *testing.T) {
	rootPEM, intermediatePEM, _, leafCert := generateTestCAChain(t)

	// Combine Root + Intermediate into single PEM string
	chainPEM := string(rootPEM) + "\n" + string(intermediatePEM)

	pool, bundle, err := LoadCertPool("", chainPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	if !strings.Contains(bundle, "Corp Root CA") || !strings.Contains(bundle, "Corp Intermediate CA") {
		// bundle should contain the PEM data
		if len(bundle) == 0 {
			t.Fatal("bundle should not be empty")
		}
	}

	// Verify that leaf cert is trusted by the pool
	intermediatePool := x509.NewCertPool()
	intermediatePool.AppendCertsFromPEM(intermediatePEM)

	opts := x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediatePool,
		DNSName:       "internal.corp.example.com",
	}

	if _, err := leafCert.Verify(opts); err != nil {
		t.Fatalf("failed to verify leaf cert against loaded pool: %v", err)
	}
}

func TestLoadCertPool_FromFile(t *testing.T) {
	rootPEM, intermediatePEM, _, leafCert := generateTestCAChain(t)

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "corporate-ca-chain.crt")
	chainData := append(rootPEM, intermediatePEM...)

	if err := os.WriteFile(certFile, chainData, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	pool, bundle, err := LoadCertPool(certFile, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bundle) == 0 {
		t.Fatal("bundle should contain loaded PEM blocks")
	}

	opts := x509.VerifyOptions{
		Roots:   pool,
		DNSName: "internal.corp.example.com",
	}
	// Add intermediate to verify
	intermediatePool := x509.NewCertPool()
	intermediatePool.AppendCertsFromPEM(intermediatePEM)
	opts.Intermediates = intermediatePool

	if _, err := leafCert.Verify(opts); err != nil {
		t.Fatalf("verification failed: %v", err)
	}
}

func TestLoadCertPool_FromDirectory(t *testing.T) {
	rootPEM, intermediatePEM, _, _ := generateTestCAChain(t)

	tmpDir := t.TempDir()

	// Write root CA to ca.crt
	if err := os.WriteFile(filepath.Join(tmpDir, "ca.crt"), rootPEM, 0644); err != nil {
		t.Fatalf("failed to write ca.crt: %v", err)
	}

	// Write intermediate CA to intermediate.pem
	if err := os.WriteFile(filepath.Join(tmpDir, "intermediate.pem"), intermediatePEM, 0644); err != nil {
		t.Fatalf("failed to write intermediate.pem: %v", err)
	}

	// Write irrelevant non-cert file
	if err := os.WriteFile(filepath.Join(tmpDir, "README.txt"), []byte("not a cert"), 0644); err != nil {
		t.Fatalf("failed to write readme: %v", err)
	}

	pool, bundle, err := LoadCertPool(tmpDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	// Both certs should be in the bundle
	if strings.Count(bundle, "-----BEGIN CERTIFICATE-----") != 2 {
		t.Fatalf("expected 2 certificates in bundle, got: %s", bundle)
	}
}

func TestLoadCertPool_NonExistentPath(t *testing.T) {
	_, _, err := LoadCertPool("/path/that/does/not/exist", "")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestGenerateSelfSignedCertificate(t *testing.T) {
	hosts := []string{"kncc.test.corp", "10.244.1.5"}
	cert, err := GenerateSelfSignedCertificate(hosts, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate self-signed certificate: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate blocks in generated cert")
	}

	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse generated certificate: %v", err)
	}

	if parsed.Subject.CommonName != "kncc-server" {
		t.Errorf("expected CN 'kncc-server', got %q", parsed.Subject.CommonName)
	}
	if parsed.NotAfter.Before(time.Now()) {
		t.Errorf("certificate should not be expired")
	}

	// Verify SANs
	dnsFound := false
	for _, dns := range parsed.DNSNames {
		if dns == "kncc.test.corp" {
			dnsFound = true
			break
		}
	}
	if !dnsFound {
		t.Errorf("expected DNSName 'kncc.test.corp' in SANs, got: %v", parsed.DNSNames)
	}

	ipFound := false
	for _, ip := range parsed.IPAddresses {
		if ip.String() == "10.244.1.5" {
			ipFound = true
			break
		}
	}
	if !ipFound {
		t.Errorf("expected IP '10.244.1.5' in SANs, got: %v", parsed.IPAddresses)
	}
}

func TestLoadServerCertificate_PEM(t *testing.T) {
	_, _, leafPEM, _ := generateTestCAChain(t)

	// Generate key for leaf
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	privBytes, _ := x509.MarshalECPrivateKey(leafKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	// To make leafPEM and keyPEM match, generate a valid pair
	selfCert, err := GenerateSelfSignedCertificate([]string{"example.com"}, time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	_ = leafPEM
	_ = keyPEM

	// Load directly from selfCert blocks
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: selfCert.Certificate[0]})
	// test invalid first
	_, err = LoadServerCertificate("", "", "", "")
	if err == nil {
		t.Fatal("expected error when no sources provided")
	}

	// test valid PEM
	privEC, ok := selfCert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("expected ECDSA private key")
	}
	ecBytes, err := x509.MarshalECPrivateKey(privEC)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	validKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecBytes})

	loaded, err := LoadServerCertificate("", "", string(certPEM), string(validKeyPEM))
	if err != nil {
		t.Fatalf("failed to load from inline PEM: %v", err)
	}
	if len(loaded.Certificate) == 0 {
		t.Fatal("expected non-empty certificate")
	}
}

func TestLoadServerCertificate_Files(t *testing.T) {
	selfCert, err := GenerateSelfSignedCertificate([]string{"files.test"}, time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: selfCert.Certificate[0]})
	privEC, ok := selfCert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("expected ECDSA private key")
	}
	ecBytes, err := x509.MarshalECPrivateKey(privEC)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecBytes})

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "tls.crt")
	keyFile := filepath.Join(tmpDir, "tls.key")

	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	loaded, err := LoadServerCertificate(certFile, keyFile, "", "")
	if err != nil {
		t.Fatalf("failed to load from files: %v", err)
	}
	if len(loaded.Certificate) == 0 {
		t.Fatal("expected non-empty certificate")
	}
}
