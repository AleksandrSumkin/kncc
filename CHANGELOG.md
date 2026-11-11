# Changelog

All notable changes to the **kncc** (Kubernetes Network Connectivity Checker) project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

## [1.15.0] - 2026-11-11

### Added
- **Switch project documentation, configuration, and API descriptions to English ([#95](https://github.com/AleksandrSumkin/kncc/issues/95)):**
  - Translated core documentation ([README.md](README.md), [GEMINI.md](GEMINI.md)) and complete version history ([CHANGELOG.md](CHANGELOG.md)) into idiomatic technical English.
  - Comprehensive English translation of all 12 configuration sections in [deploy/helm/kncc/values.yaml](deploy/helm/kncc/values.yaml), retaining Russian reference comments in `values_ru.yaml`.
  - Updated OpenAPI/Swagger root metadata and security descriptions in `cmd/kncc/main.go`, synchronizing `internal/api/docs` (`docs.go`, `swagger.json`, `swagger.yaml`).
  - Switched internal API response messages in `internal/api/favicon.go` and `internal/api/logo.go` to English, leaving Russian exclusively in dedicated UI localization files (`web/src/i18n/locales/ru.ts`).

### Fixed
- **Fix ICMP check failures with kncc-agent probe image in HostNetwork mode ([#100](https://github.com/AleksandrSumkin/kncc/issues/100)):**
  - Added `k8s.probe_allow_privilege_escalation` configuration option (default `true`) across configuration structs, environment variables (`K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION` / `KNCC_K8S_PROBE_ALLOW_PRIVILEGE_ESCALATION`), and Helm chart (`deploy/helm/kncc/values.yaml`).
  - Updated probe pod manifest builder (`internal/k8s/builder.go`) to set `AllowPrivilegeEscalation: true` by default, allowing non-root probe containers (such as `kncc-agent` running under UID 10001) to utilize the SUID bit on `/bin/ping` to acquire the `NET_RAW` capability in `HostNetwork` mode while keeping all other capabilities strictly dropped (`Drop: [ALL]`).
  - Improved probe script error handling (`internal/k8s/script.go`) to detect socket permission errors (`Operation not permitted`, `missing cap_net_raw`) and report clear diagnostic messages rather than masking them as network packet loss.
  - Hardened `Dockerfile.agent` by explicitly setting `chmod 4755 /bin/ping`.
- **Improve new check form layout and input ergonomics ([#99](https://github.com/AleksandrSumkin/kncc/issues/99)):**
  - Expanded main container width on `NewCheckPage` from `max-w-4xl` to `max-w-6xl` to align with `AdminPage` and `HistoryPage`, providing substantially more horizontal space for network targets.
  - Relocated the "Allow Insecure TLS" checkbox for HTTPS targets below the main field row with contextual helper text, preventing it from squeezing the Host/IP/FQDN input.
  - Disabled spinner stepper controls and prevented arrow-key value modifications (`ArrowUp`, `ArrowDown`, and mouse wheel scroll) on the probe timeout input field across all check types.

## [1.14.0] - 2026-11-11

### Added
- **Improved diagnostic probe duration presentation and formatting in reports ([#87](https://github.com/AleksandrSumkin/kncc/issues/87)):**
  - Implemented human-readable probe pod lifecycle duration formatting (`formatDuration`): seconds (`X.Xs`) for values ≥ 1000 ms and milliseconds for fast executions.
  - Added dedicated `Clock` badge in `NodeCard` (By Node view) with localized label ("Probe duration") and detailed tooltips in Russian and English, cleanly separating target network latency (RTT) from pod execution time.
  - Optimized probe script (`internal/k8s/script.go`): pre-validated IP addresses (`net.ParseIP`) in Go template to skip `getent hosts` / `nslookup`, avoiding reverse DNS PTR timeouts on direct IP checks.

### Fixed
- **Fix export functions in administrator interface ([#92](https://github.com/AleksandrSumkin/kncc/issues/92)):**
  - Replaced direct `navigator.clipboard.writeText` calls with safe cross-browser `copyToClipboard` utility on `AdminPage`.
  - Resolved `TypeError` crashes when copying TSV query statistics ("Copy TSV") as well as tabular ("Copy Table") and JSON representations ("Copy JSON") of K8s SA RBAC permission matrix in non-secure HTTP contexts (IP addresses, `.example.com` ingresses).
  - Added automated fallback to `document.execCommand('copy')` with asynchronous confirmation prior to displaying success feedback.

## [1.13.0] - 2026-11-10

### Added
- **Release synchronization pipeline to public GitHub and GHCR ([#73](https://github.com/AleksandrSumkin/kncc/issues/73)):**
  - Two-stage promotion pipeline for local releases via an intermediate staging repository `kncc-public`.
  - Deep sanitization module `Sanitize-Release.ps1` (redacting internal 10.0.0.* IPs, internal ports, .example.com domains, and Forgejo references).
  - Multi-arch container image index support (linux/amd64, linux/arm64) for server and `kncc-agent` in `ghcr.io`.
  - Automated bidirectional issue synchronization between GitHub Issues and local Forgejo (`Sync-GitHubIssues.ps1`).
- **Multi-language user interface support (i18n) ([#80](https://github.com/AleksandrSumkin/kncc/issues/80)):**
  - Type-safe internationalization architecture (`web/src/i18n`) with initial English (`en`) and Russian (`ru`) support.
  - Compact `LanguageSwitcher` in top navigation bar `Navbar` and login screen `LoginPage`.
  - Server-driven default language configuration via `theme.default_language` in `kncc.yaml`, `THEME_DEFAULT_LANGUAGE`, `KNCC_DEFAULT_LANGUAGE`, Helm value `theme.defaultLanguage`, and `/api/system/config`.
  - Persistent user preference in `localStorage` with automated fallback to server default and browser locale.
  - Complete localization across all views: `NewCheckPage`, `ReportPage`, `HistoryPage`, `LoginPage`, and `AdminPage`.
  - Layout-optimized tables and badges avoiding text overflows in narrow columns.
- **Comprehensive Helm chart parameter documentation in values.yaml ([#79](https://github.com/AleksandrSumkin/kncc/issues/79)):**
  - Detailed structured documentation for all parameters in `deploy/helm/kncc/values.yaml` for Zero-Docs Onboarding without external reference lookups.
  - Documented functional purposes, administrator recommendations, runtime impacts, security guidelines, and architectural constraints (Single-Writer SQLite `replicaCount: 1`, tmpfs RAM volume `auth.deliveryMode: volume`, Strict Allowlist `security.whitelist*`).
- **Comprehensive release security audit and SSDLC verification ([#86](https://github.com/AleksandrSumkin/kncc/issues/86)):**
  - Full end-to-end static and dynamic security audit: Secrets Scanning (Gitleaks), SAST (Semgrep), SCA (Trivy fs), Container Image Scan (Trivy image), DAST (Nuclei and OWASP ZAP API Scan).
  - Generated CycloneDX (`sbom.cdx.json`, `image-sbom.cdx.json`) and SPDX (`sbom.spdx.json`) software bills of materials.
  - Confirmed 0 vulnerabilities (0 CVE, 109 PASS in OWASP ZAP, 0 vulnerabilities in Nuclei) on pre-release verification stand.

## [1.12.0] - 2026-11-09

### Added
- **OpenAPI 3.x / Swagger specification publication and embedded Swagger UI ([#74](https://github.com/AleksandrSumkin/kncc/issues/74)):**
  - Automated OpenAPI specification generation from Go handler annotations (`swaggo/swag`).
  - Machine-readable specifications exposed at `/api/openapi.json` and `/api/openapi.yaml`.
  - Embedded, standalone interactive Swagger UI at `/api/docs` (with `/api/docs/` redirect) without external CDN dependencies for air-gapped Kubernetes environments.
  - Access control via `server.openapi.enabled` and `server.openapi.public` in `kncc.yaml`, Helm `values.yaml`, and environment variables:
    - Standard mode (`public: false`): documentation requires user authentication (`RequireAuth`).
    - Open mode (`public: true`): documentation accessible without token (for DAST security scanners and preview stands).
  - 100% coverage of server endpoints: health probes, system routes, authentication, checks and reports, and administrative APIs.

### Fixed
- **Content Security Policy (CSP) adjustment for Swagger UI inline scripts ([#74](https://github.com/AleksandrSumkin/kncc/issues/74)):**
  - Adjusted `Content-Security-Policy` in `SecurityHeadersMiddleware` for `/api/docs`: permitted Swagger UI initialization scripts (`'unsafe-inline'` and `'unsafe-eval'` for `script-src`, `'unsafe-inline'` for `style-src` and `font-src 'self' data:`).
  - Maintained strict security policy without `'unsafe-inline'` across all other API routes.

## [1.11.0] - 2026-11-09

### Added
- **Support for existing Kubernetes Secret for OIDC / Dex ([#67](https://github.com/AleksandrSumkin/kncc/issues/67)):**
  - Added option to source OIDC `clientID` and `clientSecret` from pre-existing Kubernetes Secret (`auth.oidc.existingSecret`).
  - Configurable secret key mapping via `auth.oidc.existingSecretClientIDKey` and `auth.oidc.existingSecretClientSecretKey`.
  - Sourced credentials via environment variables `OIDC_CLIENT_ID_FILE` / `OIDC_CLIENT_SECRET_FILE` and secret files.
  - Updated Helm manifests (`deployment.yaml`, `secret.yaml`, `configmap.yaml`, `_helpers.tpl`).
- **Additional LDAP access methods ([#66](https://github.com/AleksandrSumkin/kncc/issues/66)):**
  - Credentials from existing Kubernetes Secret (`auth.ldap.existingSecret`) and file mounts (`LDAP_BIND_DN_FILE`, `LDAP_BIND_PASSWORD_FILE`).
  - Direct Bind / User Bind Pattern (`auth.ldap.userDNTemplate`) for authenticating directly under user identity without service accounts.
  - Anonymous Bind / Unauthenticated Access (`auth.ldap.anonymousSearch`, RFC 4513) for open directories.
- **Automated administrator password generation on first startup ([#65](https://github.com/AleksandrSumkin/kncc/issues/65)):**
  - Generates cryptographically secure administrator password when none is specified via Helm values or existing secret.
  - Prints primary credentials banner into application log (stdout).
  - Default value of `auth.adminInitialPassword` set to empty string (`""`) to make auto-generation the default behavior.

### Changed
- **Default authentication method on login page ([#64](https://github.com/AleksandrSumkin/kncc/issues/64)):**
  - Defaults to "LDAP / Active Directory" on login screen (`LoginPage.tsx`) whenever LDAP provider is enabled on server.
  - Adaptive fallback to "Local Account" when LDAP is disabled, preserving user selection.

## [1.10.0] - 2026-11-09

### Added
- **Custom minimalist probe pod image (`kncc-agent`) ([#60](https://github.com/AleksandrSumkin/kncc/issues/60)):**
  - Ultra-lightweight probe image `kncc-agent` (`Dockerfile.agent`) based on Alpine Linux 3.21 (~15 MB uncompressed, ~5–6 MB compressed).
  - Minimal required diagnostics: `curl`, `openssl`, `netcat-openbsd`, `iputils-ping`, `bind-tools`, `musl-utils`, `ca-certificates`.
  - Zero-Attack-Surface: excluded background daemons, packet sniffers, port scanners, and scripting runtimes; 0 CVE vulnerabilities.
  - Runs as dedicated unprivileged user `kncc-agent` (UID/GID 10001) with write permissions to `/tmp` and minimal `NET_RAW` capability for ICMP ping.
  - Full support for TCP, UDP, HTTP, HTTPS (with X.509 certificate extraction), ICMP ping, and corporate CA integration.
  - Support for `KNCC_K8S_PROBE_IMAGE` environment variable.

## [1.9.0] - 2026-11-08

### Added
- **LDAP group-to-role mapping and `groupFilter` support ([#54](https://github.com/AleksandrSumkin/kncc/issues/54)):**
  - Configurable `auth.ldap.admin_group` (`LDAP_ADMIN_GROUP`) for granting administrator role `admin` to members of specified LDAP group.
  - Dynamic user group retrieval upon login (`AuthenticateLDAP`) and automated role sync in SQLite database (`UpdateUserRole`).
  - Added `auth.ldap.group_filter` (`LDAP_GROUP_FILTER`) and `auth.ldap.group_base_dn` (`LDAP_GROUP_BASE_DN`).
  - Support for `memberOf` attributes and nested queries (`member`, `uniqueMember`, `memberUid`).
- **Secure incoming HTTPS/TLS support on dedicated port ([#55](https://github.com/AleksandrSumkin/kncc/issues/55)):**
  - Dual-listener architecture: standard HTTP port (`8080`) serves Kubernetes health probes, while dedicated port (`8443`, `server.tls.port`) accepts encrypted TLS traffic.
  - Automated self-signed ECDSA P-256 certificate generation with SANs (DNS names, localhost, hostname, local IPs) in Zero-Configuration mode.
  - Support for custom certificates from disk (`server.tls.cert_file`), inline PEM (`server.tls.cert_pem`), or Kubernetes secrets (`existingSecret`).
  - `cert-manager` integration for automated issuance via CRD `Certificate`.

## [1.8.0] - 2026-11-08

### Added
- **Administrator password delivery via `emptyDir` volume with `medium` mode ([#47](https://github.com/AleksandrSumkin/kncc/issues/47)):**
  - Option to securely deliver administrator password via file on isolated `emptyDir` volume instead of `ADMIN_INITIAL_PASSWORD` environment variable.
  - Configurable volume `medium` (`Memory` for tmpfs in RAM), preventing secret leakage into `/proc/$PID/environ` or crash dumps.
  - Support for `auth.admin_initial_password_file` and `ADMIN_INITIAL_PASSWORD_FILE`.
- **Custom Favicon support ([#48](https://github.com/AleksandrSumkin/kncc/issues/48)):**
  - Automatic fallback: defaults to thumbnail of active corporate logo (`DefaultShipbuildingLogoSVG`).
  - Favicon provisioning via Helm values in Base64 or raw SVG (`favicon.data`, `favicon.mimeType`).
  - Public endpoints `GET /api/system/favicon` and `GET /favicon.ico` with ETag / 304 caching.
  - Admin endpoints `POST /api/admin/settings/favicon` and `DELETE /api/admin/settings/favicon`.
  - Favicon management section in `AdminPage` with browser tab preview (16x16, 32x32, 48x48) and drag-and-drop upload.
- **Trusted corporate CA certificates (Root + Intermediate) in server and probes ([#49](https://github.com/AleksandrSumkin/kncc/issues/49)):**
  - Loading internal corporate CA certificate bundles in PEM format.
  - Secure TLS connectivity from `kncc` server to LDAPS and OIDC identity providers with full certificate chain validation.
  - Automated injection of trusted CA bundle into `kncc-probe` containers merged with Alpine system trust store (`/tmp/kncc-certs/ca-bundle.crt`).

## [1.7.0] - 2026-11-07

### Added
- **Corporate logo branding ([#41](https://github.com/AleksandrSumkin/kncc/issues/41)):**
  - Configurable corporate logo in navbar, login screen, and admin settings.
  - Default vector SVG logo featuring shipbuilding and marine engineering themes (`internal/assets/default_logo.svg`).
  - Storage and synchronization in Kubernetes ConfigMap (`<release>-logo`) and SQLite fallback cache.
  - Public `GET /api/system/logo` and admin `POST /api/admin/settings/logo` / `DELETE /api/admin/settings/logo`.
- **Kubernetes startup, liveness, and readiness probes ([#39](https://github.com/AleksandrSumkin/kncc/issues/39)):**
  - Implemented `/api/livez`, `/api/readyz`, `/api/startupz`, and alias `/api/healthz`.
  - Active SQLite database validation (`store.Ping`) returning HTTP 503 on degradation without crashing the container.
  - Helm chart `probes` section in `values.yaml` for tuning probe timings.

### Fixed
- **Application version display in UI and build pipeline ([#40](https://github.com/AleksandrSumkin/kncc/issues/40)):**
  - Resolved forced override to `dev` in `Dockerfile` when building without `--build-arg`.
  - Added `APP_VERSION` environment variable support and normalized footer version formatting.

## [1.6.0] - 2026-11-06

### Added
- **Quick-add diagnostic target presets on new check page ([#32](https://github.com/AleksandrSumkin/kncc/issues/32)):**
  - Configurable presets (1 to 3 services) via `quick_add.presets` and Helm `quickAdd.presets`.
  - Persistent request history in SQLite table `user_target_history` with automatic backfill.
  - Protected API endpoint `GET /api/checks/quick-add` dynamically returning most frequent targets.
- **Enhanced report link copying and footer version display ([#36](https://github.com/AleksandrSumkin/kncc/pulls/36)):**
  - Cross-browser `clipboard.ts` module with Navigator Clipboard API, fallback to `document.execCommand('copy')`, and dialog fallback for HTTP contexts.
  - Toast notifications on `ReportPage.tsx`.
- **External secret support for administrator password ([#29](https://github.com/AleksandrSumkin/kncc/issues/29)):**
  - Sourcing administrator password from existing Kubernetes Secret (`auth.existingSecret`, `auth.existingSecretPasswordKey`).

## [1.5.0] - 2026-11-06

### Added
- **User request analytics and statistics in administrator dashboard ([#28](https://github.com/AleksandrSumkin/kncc/issues/28)):**
  - "Query Statistics" tab in `AdminPage` tracking check executions per user over 24h, 7d, 30d, 365d, and all-time.
  - Summary KPI cards, interactive search, column sorting, and TSV export.
  - Persistent lightweight audit table `user_request_logs` in SQLite.
  - Protected API endpoint `GET /api/admin/stats/users`.

## [1.4.0] - 2026-11-06

### Added
- **Kubernetes Gateway API support ([#19](https://github.com/AleksandrSumkin/kncc/issues/19)):**
  - Implemented `HTTPRoute` manifest (`templates/httproute.yaml`) under `gateway.networking.k8s.io/v1`.
  - Support for external gateways via `gateway.parentRefs` (Traefik, Cilium, Envoy Gateway, Istio).
  - Optional chart-managed `Gateway` resource (`gateway.createGateway: true`).
- **Network mode indicator on report page ([#20](https://github.com/AleksandrSumkin/kncc/issues/20)):**
  - Contrast visual indicator for execution network mode (`Host Network` vs `Pod Network`) with namespace chip (`ns: <namespace>`).
- **Dark UI theme support ([#15](https://github.com/AleksandrSumkin/kncc/issues/15)):**
  - Instant theme switching (Light / Dark) in `Navbar` and login screen with `localStorage` persistence.
  - Comprehensive dark mode styling across all views.
  - System-wide default theme configuration via YAML (`theme.default_theme`), env (`THEME_DEFAULT_THEME`), and Helm (`theme.defaultTheme`).

### Changed
- **Documentation and demo seeding cleanup ([#21](https://github.com/AleksandrSumkin/kncc/issues/21)):**
  - Removed deprecated `developer` demo account and re-associated demo report with `admin`.

### Security
- **HTTP Security Headers enforcement via OWASP ZAP DAST audit ([#16](https://github.com/AleksandrSumkin/kncc/issues/16)):**
  - Centralized `SecurityHeadersMiddleware`: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `X-XSS-Protection: 1; mode=block`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy`, strict CSP.
  - Tightened `CORSMiddleware` removing unrestricted `Access-Control-Allow-Origin: *` on static assets.

## [1.3.0] - 2026-11-05

### Added
- **ServiceAccount Kubernetes API RBAC permission audit and dashboard ([#13](https://github.com/AleksandrSumkin/kncc/issues/13)):**
  - Formatted ASCII table of effective permissions logged on startup via `rbac-tool policy-rules` standard.
  - Two-tier permission engine (`internal/k8s/rbac`) with `SelfSubjectRulesReview` and binding traversal.
  - Admin endpoint `GET /api/admin/rbac` and interactive dashboard on `AdminPage.tsx`.
- **SSL/TLS certificate expiration auditing for HTTPS targets ([#9](https://github.com/AleksandrSumkin/kncc/issues/9)):**
  - Automated certificate metadata collection via OpenSSL in probe pod.
  - Three-tier status badges in reports: Valid (>30 days), Expiring soon (≤30 days), Expired (≤0 days).
- **Configurable Garbage Collector stale threshold ([#5](https://github.com/AleksandrSumkin/kncc/issues/5)):**
  - Added `k8s.gc_stale_threshold` (`K8S_GC_STALE_THRESHOLD`, Helm `k8s.gcStaleThreshold`, default `3m`).
- **"Home" action on report error view ([#3](https://github.com/AleksandrSumkin/kncc/issues/3)).**

### Security
- **Patched CVE-2026-116587 (go-jose/go-jose):** Upgraded `github.com/go-jose/go-jose/v4` to `v4.1.6`.
- **Restricted admin view access for standard users ([#11](https://github.com/AleksandrSumkin/kncc/issues/11)):** Route Guard in `App.tsx` and `AdminPage.tsx`.

### Changed
- **Streamlined check execution button label ([#6](https://github.com/AleksandrSumkin/kncc/issues/6)):** Renamed to `Run Check`.

### Fixed
- **Resolved race condition in probe pod garbage collector ([#5](https://github.com/AleksandrSumkin/kncc/issues/5)):** Guaranteed retention threshold before deletion.

## [1.2.0] - 2026-11-04

### Added
- **Target-Centric Report View:**
  - "By Target" primary perspective with aggregate status badges (`ALL NODES REACHABLE`, `PARTIALLY REACHABLE`, `UNREACHABLE`).
  - Interactive node status chips and detailed expandable tables.
  - Bulk actions: **Expand all** / **Collapse all**, status filters (**All**, **Issues Only**, **All Reachable**).
- **"Back to Edit" action:**
  - One-click return from report to check creation form with pre-populated parameters.
- **Two-row report action toolbar.**

### Fixed
- **Probe script JSON formatting (Backend):** Eliminated spurious whitespace in empty error output.
- **Mode toggle layout stability.**

## [1.1.0] - 2026-11-04

### Added
- **Target Namespace dropdown:** Interactive cluster namespace selector with toggle to manual input.
- **Namespace listing configuration toggle:** `k8s.namespace_list_enabled` (default `false`).
- **System and sensitive namespace filtering:** `k8s.namespace_exclude_list` and `k8s.namespace_exclude_selector`.
- **REST API:** `GET /api/k8s/namespaces` endpoint.
- **Admin dashboard policy indicators.**

### Security
- **Principle of Least Privilege in RBAC:** Conditional `namespaces: ["get", "list", "watch"]` permissions in ClusterRole.
- **Anti-Bypass validation in API:** Server-side enforcement of allowed namespaces on `POST /api/checks`.

## [1.0.0] - 2026-11-02

### Initial Release
- **Parallel Network Diagnostics:** On-demand pods scheduled across all cluster nodes concurrently with system `Tolerations`.
- **Diagnostic Protocols & Modes:** TCP handshake, UDP tests, HTTP/HTTPS validation, ICMP ping; Host Network and Pod Network modes.
- **Security & Abuse Prevention:** Anti-SSRF blacklists, DNS rebinding validation, per-user rate limiting.
- **Authentication & RBAC:** Local SQLite credentials, LDAP / Active Directory, and OpenID Connect SSO.
- **Web UI & Theming:** React 18, Vite, Tailwind CSS SPA embedded into Go binary with custom color palettes.
- **Storage & Deployment:** SQLite with WAL mode, production-ready Helm chart (`deploy/helm/kncc`), multi-stage Dockerfile and Kaniko support.
