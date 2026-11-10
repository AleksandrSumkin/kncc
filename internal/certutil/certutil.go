package certutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CertInfo represents human-readable metadata of a loaded CA certificate.
type CertInfo struct {
	Subject    string    `json:"subject"`
	Issuer     string    `json:"issuer"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	IsCA       bool      `json:"is_ca"`
	CommonName string    `json:"common_name"`
}

// LoadCertPool loads the system root CA pool and appends custom CA certificates
// from a file, directory, or inline PEM string.
// It returns the combined *x509.CertPool and the concatenated PEM string of all custom CAs.
func LoadCertPool(customCAPath string, customPEM string) (*x509.CertPool, string, error) {
	// 1. Initialize base cert pool from system roots
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		log.Printf("Notice: could not load system cert pool (%v), initializing empty pool", err)
		pool = x509.NewCertPool()
	}

	var pemBlocks [][]byte

	// 2. Load certificates from path (file or directory)
	if customCAPath != "" {
		fileBlocks, err := readPEMFromPath(customCAPath)
		if err != nil {
			return nil, "", fmt.Errorf("reading custom CA from path %q: %w", customCAPath, err)
		}
		pemBlocks = append(pemBlocks, fileBlocks...)
	}

	// 3. Load certificates from inline PEM string
	if strings.TrimSpace(customPEM) != "" {
		inlineBlocks := extractPEMBlocks([]byte(customPEM))
		pemBlocks = append(pemBlocks, inlineBlocks...)
	}

	if len(pemBlocks) == 0 {
		return pool, "", nil
	}

	// 4. Parse, validate, and add certificates to the pool
	var combinedPEM strings.Builder
	loadedCount := 0

	for i, rawPEM := range pemBlocks {
		block, _ := pem.Decode(rawPEM)
		if block == nil {
			continue
		}
		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			log.Printf("Warning: failed to parse certificate block #%d: %v", i+1, err)
			continue
		}

		pool.AddCert(cert)
		combinedPEM.WriteString(string(rawPEM))
		if !strings.HasSuffix(string(rawPEM), "\n") {
			combinedPEM.WriteString("\n")
		}
		loadedCount++

		log.Printf("Loaded custom trusted CA: CN=%q, Issuer=%q, Expires=%s, IsCA=%v",
			cert.Subject.CommonName,
			cert.Issuer.CommonName,
			cert.NotAfter.UTC().Format(time.RFC3339),
			cert.IsCA,
		)
	}

	if loadedCount == 0 && (customCAPath != "" || strings.TrimSpace(customPEM) != "") {
		log.Printf("Warning: custom CA source was specified, but no valid X.509 CERTIFICATE blocks were loaded")
	} else {
		log.Printf("Successfully loaded %d custom CA certificate(s)", loadedCount)
	}

	return pool, combinedPEM.String(), nil
}

func readPEMFromPath(path string) ([][]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", path)
		}
		return nil, err
	}

	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return extractPEMBlocks(data), nil
	}

	// If directory, inspect all files
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var allBlocks [][]byte
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		// Match certificate extensions or cert-manager/K8s standard secret keys
		if strings.HasSuffix(name, ".crt") ||
			strings.HasSuffix(name, ".pem") ||
			strings.HasSuffix(name, ".cer") ||
			strings.HasPrefix(name, "ca") ||
			strings.HasPrefix(name, "tls") {

			filePath := filepath.Join(path, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Warning: failed to read certificate file %s: %v", filePath, err)
				continue
			}
			blocks := extractPEMBlocks(data)
			allBlocks = append(allBlocks, blocks...)
		}
	}

	return allBlocks, nil
}

func extractPEMBlocks(data []byte) [][]byte {
	var blocks [][]byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			encoded := pem.EncodeToMemory(block)
			if len(encoded) > 0 {
				blocks = append(blocks, encoded)
			}
		}
	}
	return blocks
}

// GenerateSelfSignedCertificate generates a self-signed X.509 certificate and private key
// using ECDSA (P-256) curve, suitable for immediate zero-config TLS termination.
func GenerateSelfSignedCertificate(hosts []string, validFor time.Duration) (tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating ECDSA key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating serial number: %w", err)
	}

	if validFor <= 0 {
		validFor = 365 * 24 * time.Hour
	}
	notBefore := time.Now().Add(-10 * time.Minute)
	notAfter := notBefore.Add(validFor)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"kncc"},
			CommonName:   "kncc-server",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	dnsMap := map[string]struct{}{
		"localhost":                     {},
		"kncc":                          {},
		"kncc.kncc.svc":                 {},
		"kncc.kncc.svc.cluster.local":   {},
		"kncc.default.svc":              {},
		"kncc.default.svc.cluster.local": {},
	}
	ipMap := map[string]net.IP{
		"127.0.0.1": net.ParseIP("127.0.0.1"),
		"::1":       net.ParseIP("::1"),
	}

	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		dnsMap[hostname] = struct{}{}
	}

	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ip := ipNet.IP; ip != nil {
					ipMap[ip.String()] = ip
				}
			}
		}
	}

	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			ipMap[ip.String()] = ip
		} else {
			dnsMap[strings.ToLower(host)] = struct{}{}
		}
	}

	for dns := range dnsMap {
		template.DNSNames = append(template.DNSNames, dns)
	}
	for _, ip := range ipMap {
		template.IPAddresses = append(template.IPAddresses, ip)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("creating self-signed certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshaling EC private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parsing generated key pair: %w", err)
	}

	log.Printf("Generated self-signed TLS certificate: CN=%q, Expires=%s, DNSNames=%d, IPAddresses=%d",
		template.Subject.CommonName,
		template.NotAfter.UTC().Format(time.RFC3339),
		len(template.DNSNames),
		len(template.IPAddresses),
	)

	return tlsCert, nil
}

// LoadServerCertificate loads a server TLS certificate from file paths or inline PEM strings.
// It parses and audits the certificate metadata (expiration, SANs).
func LoadServerCertificate(certPath, keyPath, certPEM, keyPEM string) (tls.Certificate, error) {
	var (
		tlsCert tls.Certificate
		err     error
	)

	if certPath != "" && keyPath != "" {
		tlsCert, err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("loading key pair from files (%s, %s): %w", certPath, keyPath, err)
		}
	} else if strings.TrimSpace(certPEM) != "" && strings.TrimSpace(keyPEM) != "" {
		tlsCert, err = tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("loading key pair from inline PEM: %w", err)
		}
	} else {
		return tls.Certificate{}, errors.New("no certificate and private key provided (paths or PEM blocks are required)")
	}

	if len(tlsCert.Certificate) > 0 {
		cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err == nil {
			log.Printf("Loaded custom server TLS certificate: CN=%q, Issuer=%q, Expires=%s, DNSNames=%v, IPs=%v",
				cert.Subject.CommonName,
				cert.Issuer.CommonName,
				cert.NotAfter.UTC().Format(time.RFC3339),
				cert.DNSNames,
				cert.IPAddresses,
			)
			if time.Now().After(cert.NotAfter) {
				log.Printf("WARNING: server TLS certificate is EXPIRED (expired on %s)", cert.NotAfter.UTC().Format(time.RFC3339))
			} else if time.Until(cert.NotAfter) < 30*24*time.Hour {
				log.Printf("Notice: server TLS certificate will expire soon (in %v)", time.Until(cert.NotAfter).Round(time.Hour))
			}
		}
	}

	return tlsCert, nil
}
