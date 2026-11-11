package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"kncc/internal/api"
	"kncc/internal/auth"
	"kncc/internal/certutil"
	"kncc/internal/config"
	"kncc/internal/k8s"
	"kncc/internal/k8s/rbac"
	"kncc/internal/ratelimit"
	"kncc/internal/security"
	"kncc/internal/store"
	"kncc/web"

	"k8s.io/client-go/kubernetes"
)

// @title Kubernetes Network Connectivity Checker (kncc) API
// @version 1.15.0
// @description kncc server REST API for network connectivity diagnostics in Kubernetes clusters.
// @contact.name kncc Maintainers
// @license.name Apache 2.0
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter 'Bearer <JWT_TOKEN>' to authenticate protected requests.

var (
	version = "1.15.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "", "Path to configuration YAML file")
	mockMode := flag.Bool("mock", false, "Run in mock/demo mode with simulated cluster nodes and sample reports")
	flag.Parse()

	if envVer := os.Getenv("APP_VERSION"); envVer != "" {
		version = envVer
	} else if envVer := os.Getenv("KNCC_VERSION"); envVer != "" {
		version = envVer
	}

	log.Printf("Starting kncc (%s, commit: %s, built: %s)", version, commit, date)

	// 1. Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 1b. Load custom trusted CA certificates (if configured)
	certPool, customCABundle, err := certutil.LoadCertPool(cfg.CACerts.Path, cfg.CACerts.PEM)
	if err != nil {
		log.Printf("Warning: failed to load custom CA certificates: %v", err)
	} else if customCABundle != "" {
		if cfg.K8s.CustomCABundle == "" {
			cfg.K8s.CustomCABundle = customCABundle
		}
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			if tr.TLSClientConfig == nil {
				tr.TLSClientConfig = &tls.Config{}
			}
			tr.TLSClientConfig.RootCAs = certPool
		}
	}

	// 2. Initialize SQLite storage
	dbStore, err := store.NewSQLiteStore(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database (%s): %v", cfg.Storage.DBPath, err)
	}
	defer func() {
		if err := dbStore.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// 3. Ensure default admin user
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	existingAdmin, err := dbStore.GetUserByUsername(ctx, cfg.Auth.AdminUsername)
	if err == nil && existingAdmin != nil {
		log.Printf("Admin user '%s' already exists in database; skipping initial password setup", cfg.Auth.AdminUsername)
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("Warning: failed to verify existing admin user '%s': %v", cfg.Auth.AdminUsername, err)
	} else {
		adminPassword := cfg.Auth.AdminInitialPassword
		generated := false
		if adminPassword == "" {
			adminPassword = auth.GenerateSecurePassword(16)
			generated = true
		}

		if err := dbStore.EnsureAdminUser(ctx, cfg.Auth.AdminUsername, adminPassword); err != nil {
			log.Printf("Warning: failed to ensure admin user: %v", err)
		} else {
			if generated {
				log.Println("==================================================================")
				log.Println("  INITIAL ADMIN CREDENTIALS GENERATED:")
				log.Printf("    Username: %s\n", cfg.Auth.AdminUsername)
				log.Printf("    Password: %s\n", adminPassword)
				log.Println("  Please store these credentials safely and change upon first login.")
				log.Println("==================================================================")
			} else {
				log.Printf("Admin user '%s' initialized with configured initial password", cfg.Auth.AdminUsername)
			}
		}
	}
	cancel()

	// 4. Initialize Auth Service & JWT Manager
	jwtMgr := auth.NewJWTManager(cfg.Server.JWTSecret, cfg.Server.TokenTTL)
	authSvc, err := auth.NewAuthService(context.Background(), &cfg.Auth, dbStore, jwtMgr, certPool)
	if err != nil {
		log.Fatalf("Failed to initialize auth service: %v", err)
	}
	log.Printf("Auth providers enabled: %v", authSvc.GetEnabledProviders())

	// 5. Initialize Security Validator & Rate Limiter
	validator, err := security.NewTargetValidator(&cfg.Security)
	if err != nil {
		log.Fatalf("Failed to initialize target validator: %v", err)
	}

	limiter := ratelimit.NewLimiter(cfg.Security.RateLimitPerMinute)
	defer limiter.Stop()

	// 6. Initialize Kubernetes client & Runner
	var k8sClient kubernetes.Interface
	k8sClient, err = k8s.NewClient(cfg.K8s.Kubeconfig)
	if err != nil {
		log.Printf("Notice: Kubernetes client could not connect (%v). Running in mock/standalone mode.", err)
	} else {
		log.Printf("Connected to Kubernetes cluster successfully")
	}

	stopCh := make(chan struct{})

	// Background TTL report cleaner
	go runTTLReportCleaner(dbStore, 10*time.Minute, stopCh)

	// Kubernetes Pod Garbage Collector
	if k8sClient != nil {
		go k8s.StartGarbageCollector(context.Background(), k8sClient, cfg.K8s.Namespace, cfg.K8s.MultiNamespaceAllowed, cfg.K8s.GCInterval, cfg.K8s.GCStaleThreshold, stopCh)
	}

	var runner api.CheckRunner
	if k8sClient != nil && !*mockMode {
		runner = k8s.NewRunner(k8sClient, dbStore, cfg)
	} else {
		log.Printf("Using MockRunner (simulating 4 cluster nodes: control-plane-01, worker-01, worker-02, worker-03)")
		runner = k8s.NewMockRunner(dbStore, cfg)
		if err := k8s.SeedDemoData(dbStore, cfg); err != nil {
			log.Printf("Notice: seeding demo data: %v", err)
		} else {
			log.Printf("Demo reports and users seeded into database")
		}
	}

	// 6b. Evaluate and log Kubernetes ServiceAccount RBAC permissions
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	rbacReport := rbac.InspectCurrentPermissions(inspectCtx, k8sClient, cfg.K8s.Namespace, *mockMode || k8sClient == nil)
	inspectCancel()

	log.Printf("Kubernetes ServiceAccount RBAC inspection complete (subject: %s %s, source: %s, rules: %d)",
		rbacReport.Subject.Kind, rbacReport.Subject.Name, rbacReport.Source, len(rbacReport.Rules))
	log.Printf("Resulting RBAC policy rules:\n%s", rbac.FormatPolicyRulesTable(rbacReport))

	// 7. Setup HTTP API Router with embedded frontend UI
	webFS, err := web.GetFS()
	if err != nil {
		log.Printf("Notice: failed to load embedded frontend: %v", err)
	} else {
		log.Printf("Embedded frontend UI loaded successfully")
	}

	handler := api.NewHandler(cfg, dbStore, authSvc, validator, limiter, runner, k8sClient, version)
	router := api.SetupRouter(handler, authSvc, limiter, webFS)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 8. Start HTTP Server
	go func() {
		log.Printf("kncc HTTP API listening on http://%s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 8b. Start HTTPS Server (if enabled on dedicated port)
	var tlsServer *http.Server
	if cfg.Server.TLS.Enabled {
		var (
			serverCert tls.Certificate
			certErr    error
		)

		hasCustomCert := (cfg.Server.TLS.CertFile != "" && cfg.Server.TLS.KeyFile != "") ||
			(cfg.Server.TLS.CertPEM != "" && cfg.Server.TLS.KeyPEM != "")

		if hasCustomCert {
			serverCert, certErr = certutil.LoadServerCertificate(
				cfg.Server.TLS.CertFile,
				cfg.Server.TLS.KeyFile,
				cfg.Server.TLS.CertPEM,
				cfg.Server.TLS.KeyPEM,
			)
			if certErr != nil {
				log.Fatalf("Failed to load custom server TLS certificate: %v", certErr)
			}
		} else if cfg.Server.TLS.AutoGenerate {
			log.Printf("No custom server TLS certificate provided, generating self-signed certificate...")
			serverCert, certErr = certutil.GenerateSelfSignedCertificate(nil, 365*24*time.Hour)
			if certErr != nil {
				log.Fatalf("Failed to generate self-signed TLS certificate: %v", certErr)
			}
		} else {
			log.Fatalf("Server TLS is enabled, but no certificate is provided and auto_generate is false")
		}

		minVersion := uint16(tls.VersionTLS12)
		if cfg.Server.TLS.MinVersion == "1.3" || strings.EqualFold(cfg.Server.TLS.MinVersion, "tls1.3") {
			minVersion = tls.VersionTLS13
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			MinVersion:   minVersion,
		}

		tlsAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.TLS.Port)
		tlsServer = &http.Server{
			Addr:         tlsAddr,
			Handler:      router,
			TLSConfig:    tlsConfig,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		go func() {
			log.Printf("kncc HTTPS API listening on https://%s", tlsAddr)
			if err := tlsServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTPS server error: %v", err)
			}
		}()
	}

	log.Printf("kncc initialization complete. Namespace: %s, ProbeImage: %s, MultiNamespace: %v",
		cfg.K8s.Namespace, cfg.K8s.ProbeImage, cfg.K8s.MultiNamespaceAllowed)
	log.Printf("Security: %d Blacklist CIDRs, %d Domains, RateLimit: %d req/min",
		len(cfg.Security.BlacklistCIDRs), len(cfg.Security.BlacklistDomains), cfg.Security.RateLimitPerMinute)

	// Graceful shutdown listener
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	sig := <-sigCh
	log.Printf("Received signal %v, initiating graceful shutdown...", sig)
	close(stopCh)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()
	if tlsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tlsServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("HTTPS server shutdown error: %v", err)
			}
		}()
	}
	wg.Wait()
	log.Println("kncc server stopped cleanly.")
}

func runTTLReportCleaner(s store.Store, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			deleted, err := s.DeleteExpiredReports(ctx)
			cancel()
			if err != nil {
				log.Printf("Error running report TTL cleaner: %v", err)
			} else if deleted > 0 {
				log.Printf("Report TTL cleaner removed %d expired reports", deleted)
			}
		case <-stop:
			return
		}
	}
}
