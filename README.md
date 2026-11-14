# Kubernetes Network Connectivity Checker (`kncc`)

<p align="center">
  <strong>A lightweight, open-source parallel network connectivity diagnostic tool for Kubernetes</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/React-18-61DAFB?style=flat&logo=react" alt="React" />
  <img src="https://img.shields.io/badge/TailwindCSS-3.4-38B2AC?style=flat&logo=tailwind-css" alt="Tailwind CSS" />
  <img src="https://img.shields.io/badge/Kubernetes-1.26+-326CE5?style=flat&logo=kubernetes" alt="Kubernetes" />
  <img src="https://img.shields.io/badge/License-Apache_2.0-blue.svg" alt="License" />
</p>

---

## 1. Problem Statement & Project Purpose

In enterprise environments and multi-tenant Kubernetes clusters with strict network segmentation (firewalls, CNI NetworkPolicies, Calico, Cilium), regular developers and support engineers **lack direct node access and are restricted from running `kubectl exec` into containers**.

During application deployment, scenarios frequently arise where an external service (database, corporate API, message broker) is unreachable from specific cluster nodes or target namespaces. Applications crash on startup with cryptic network errors, prolonging troubleshooting and posing business risks.

**kncc** solves this challenge:
- Empowers users and support engineers through an intuitive web UI and REST API to run connectivity checks against external targets **in parallel across all cluster nodes** (with a single click of the **"Run Check"** button).
- Provides a **Target-Centric Report View** with aggregate reachability statuses (`ALL NODES REACHABLE`, `PARTIALLY REACHABLE`, `UNREACHABLE`), interactive node chips, and a instant **Back to Edit** action.
- Supports two key diagnostic modes: **Host Network** (verifies physical node routing and perimeter firewalls) and **Pod Network** (verifies CNI overlays and `NetworkPolicy` rules within a target namespace).
- Supports safe namespace selection via interactive cluster dropdown or manual input with automatic exclusion of system namespaces.
- Enables persistent link sharing for diagnostic reports with colleagues and network administrators, as well as one-click test reruns.

The complete version history is documented in **[CHANGELOG.md](CHANGELOG.md)**.

---

## 2. Architecture & How It Works

```text
                      +---------------------------------------+
                      |                Browser                |
                      |   React 18 + Vite + Tailwind CSS SPA  |
                      +-------------------+-------------------+
                                          |
                                HTTP REST API / WebSocket
                                          v
+---------------------------------------------------------------------------------+
| kncc Server (Go Pod)                                                            |
|                                                                                 |
|   +-------------------+    +--------------------+    +----------------------+   |
|   |  Security Engine  |    |    Auth Service    |    |   SQLite Database    |   |
|   | Anti-SSRF / CIDR  |    | Local, LDAP, OIDC  |    |  WAL, Reports, TTL   |   |
|   +-------------------+    +--------------------+    +----------------------+   |
|                                     |                                           |
|                              Runner | K8s API                                   |
+-------------------------------------|-------------------------------------------+
                                      |
         Concurrent execution of On-Demand Pods on every cluster node
                                      |
        +-----------------------------+-----------------------------+
        |                             |                             |
        v                             v                             v
+----------------+            +----------------+            +----------------+
|  Node: worker1 |            |  Node: worker2 |            | Node: control1 |
| +------------+ |            | +------------+ |            | +------------+ |
| | Probe Pod  | |            | | Probe Pod  | |            | | Probe Pod  | |
| |(kncc-agent)| |            | |(kncc-agent)| |            | |(kncc-agent)| |
| +------------+ |            | +------------+ |            | +------------+ |
+----------------+            +----------------+            +----------------+
        |                             |                             |
        +-----------------------------+-----------------------------+
                                      v
       Diagnostic output written to /dev/termination-log (JSON) -> Report
```

### Key Architectural Principles:
1. **On-Demand Pods:** Zero long-running daemon agents. Pods are spun up strictly for the duration of a check and immediately cleaned up by the backend.
2. **Guaranteed Scheduling on All Nodes:** System `Tolerations` (`operator: Exists`) ensure execution across master/control-plane and tainted nodes (GPU, dedicated node pools).
3. **Fast and Reliable Result Gathering:** Each probe writes structured JSON containing latency, DNS resolution, and error details to `/dev/termination-log`. The backend extracts results directly from Pod API termination status without streaming logs.
4. **Autonomous Garbage Collector:** Background GC worker periodically discovers and purges finished or abandoned `kncc-probe` pods with age safeguards (`k8s.gcStaleThreshold`) across all namespaces in Multi-Namespace mode.

---

## 3. Supported Protocols and Network Modes

### Protocols:
* **TCP:** Validates 3-way handshake (SYN/ACK). Clearly distinguishes between closed ports (*Connection refused RST*) and firewall drops (*Connection timed out*).
* **UDP:** Sends test datagrams and validates port responsiveness without ICMP Port Unreachable responses.
* **HTTP / HTTPS:** Measures latency, validates HTTP status codes (2xx/3xx), with optional TLS certificate validation bypass (`insecure_tls`). For **HTTPS**, automatically collects and displays certificate validity dates, remaining days, subject, and issuer, flagging expiring certificates (<=30 days) and expired certificates.
* **ICMP (Ping):** Measures average round-trip time (RTT latency) and packet loss percentage.

### Network Check Modes:
* **`Host Network` (`hostNetwork: true`):** Runs inside the host network namespace of the node. Validates physical node routing, default gateway, and corporate perimeter firewalls.
* **`Pod Network`:** Runs inside the CNI overlay network in a user-specified `Namespace`. Identifies traffic blocks enforced by `NetworkPolicy` controllers (Calico, Cilium, kube-router).

#### Target Namespace Selection (Dropdown or Manual Input):
- **Safe Mode by Default (`k8s.namespace_list_enabled: false`):** Users enter namespaces manually. kncc ServiceAccount is granted no cluster-wide namespace read permissions (Principle of Least Privilege).
- **Cluster List Mode (`k8s.namespace_list_enabled: true`):** Administrators allow kncc to query the Kubernetes API and present a searchable namespace dropdown in the UI.
- **Filtering & Blacklisting:**
  - System namespaces (`kube-system`, `kube-public`, `kube-node-lease`, or custom entries in `namespace_exclude_list`) are hidden from selection.
  - Supports `namespace_exclude_selector` (LabelSelector): Administrators can tag sensitive or restricted namespaces (e.g. `kncc.io/exclude=true`) to hide them automatically.
  - **Anti-Bypass Protection:** Backend API (`POST /api/checks`) rejects any attempts to schedule probe pods in excluded namespaces, even if requests bypass the web UI.

#### Node Blacklisting & Selection:
- **Node Blacklist (`k8s.node_exclude_list` & `k8s.node_exclude_selector`):** Administrators can exclude specific nodes (e.g. control-plane, audit, or dedicated security infrastructure nodes) by exact name or using a Kubernetes `LabelSelector` (e.g. `node-role.kubernetes.io/control-plane=true` or `security.zone=audit`).
- **Node Whitelist (`k8s.node_selector`):** Optionally restrict probe pod scheduling to nodes matching a specific label selector.
- **Silent Exclusion:** During check execution, excluded nodes are omitted prior to probe pod creation. The report's node count accurately reflects only eligible nodes without alerting end-users or interrupting workflow.

---

### 3.3. Report Analysis (Target-Centric Reporting & Back to Edit)

Connectivity reports offer two complementary visualization modes:

- **Grouped by Target (`By Target`, default):**
  - Aggregates results around target endpoints rather than individual cluster nodes.
  - Instant cluster-wide reachability status badges:
    - **`ALL NODES REACHABLE`** — target reachable from 100% of cluster nodes.
    - **`PARTIALLY REACHABLE`** — partial network isolation (reachable from some nodes, blocked on others).
    - **`UNREACHABLE`** — completely unreachable across all nodes.
  - Interactive mini status chips (`✓ node`, `✗ node`) in the target card header.
  - Detailed node breakdown displays node role badges (`control-plane`, `worker`, etc.) derived from node labels, matching `kubectl get nodes` conventions (omitted when no roles are assigned).
  - Target cards collapsed by default for compact navigation when checking dozens of endpoints.
  - Bulk actions: **Expand all** / **Collapse all**.
  - Quick status filters: `All`, `Issues Only` (focuses solely on problematic targets), and `All Reachable`.
  - Full-text search across service names, IP addresses, node names, and node roles.

- **Grouped by Node (`By Node`):**
  - Classic node-oriented perspective auditing connectivity relative to physical cluster nodes.
  - Node role badges (`control-plane`, `worker`, custom roles) displayed directly in the node card header (omitted when no roles are assigned).
  - Clear presentation of probe execution time (`Probe runtime`) in seconds (`X.Xs`) or milliseconds (`X ms`) with informative tooltips and timer icons, separated from target network latency (RTT).
  - Optimized IP probing bypassing reverse DNS PTR lookup timeouts.

- **Check Mode Marker:**
  - High-contrast visual badge (**`Host Network`** or **`Pod Network`** with `ns: <namespace>`) displayed in the results toolbar next to the view toggle and in the report header.
  - Informs operators immediately whether diagnostics were executed on host interfaces (`hostNetwork: true`) or within an isolated CNI pod network with `NetworkPolicy`.
  - Supports light and dark UI themes with explanatory tooltips.

- **Return to Edit Form (`Back to Edit`):**
  - One-click button returning to the check creation form with parameters pre-populated from the active report (network mode, target namespace, endpoint list, ports, protocols, timeouts).
  - Enables instant typo corrections or target adjustments without re-entering data.
  - Backed by `sessionStorage` caching with a convenient **Reset Form** banner.

---

### 3.4. Trusted Corporate Certificates (Private CA & cert-manager)

In enterprise environments, internal services are issued by private Certificate Authorities (Root CA and Intermediate CAs).
`kncc` provides end-to-end trust for private enterprise certificates:
1. **kncc Server Container:**
   - Outbound TLS connections to corporate **LDAP / LDAPS** directories (`ldaps://...`) and **OIDC** providers (Keycloak, Dex, ADFS) validate full trust chains without requiring insecure `insecure_skip_verify: true`.
2. **kncc-probe Containers:**
   - Automatically receive the trusted CA bundle (combining Alpine system CA with corporate trust chains).
   - `curl` and `openssl` securely validate internal HTTPS cluster endpoints without requiring `insecure_tls: true`.
   - Supported across all modes: Pod Network, Host Network, and Multi-Namespace.
3. **Provisioning Methods via Helm (`deploy/helm/kncc`):**
   - **Inline PEM:** Embed certificate chains directly into `values.yaml` (`caCerts.pem: | ...`).
   - **Existing Secret / ConfigMap:** Link pre-provisioned resources (`caCerts.existingSecret` or `caCerts.existingConfigMap`).
   - **cert-manager Integration:** Automatically provision a `cert-manager.io/v1` `Certificate` bound to an enterprise `ClusterIssuer` (`caCerts.certManager.enabled: true`).

---

### 3.5. Probe Pod Images

When running network diagnostics, `kncc` schedules ephemeral `kncc-probe` pods on designated cluster nodes. Probe containers must package sufficient network tools to execute diagnostics (TCP, UDP, HTTP, HTTPS, ICMP) and output structured JSON to `/dev/termination-log`.

#### 1. Custom Specialized `kncc-agent` Image (Recommended)
To minimize network overhead, speed up cold-starts, and achieve a strict **Zero-Attack-Surface**, `kncc` provides its own specialized probe image:
- **Base image:** `alpine:3.21`
- **Footprint:** only **~15 MB** uncompressed (~5–6 MB compressed download), 8–10x smaller than public multitool images.
- **Included utilities:** strictly tailored to `kncc` requirements:
  - `curl` — latency measurements, status codes, and HTTP requests;
  - `openssl` — detailed X.509 certificate inspection (expiration dates, Issuer, Subject);
  - `netcat-openbsd` — reliable TCP and UDP port probing (`nc -z -w`, `nc -u -z -w`);
  - `iputils-ping` — average RTT and ICMP packet loss calculations;
  - `bind-tools` and `musl-utils` — DNS name resolution (`dig`, `nslookup`, `getent hosts`);
  - `ca-certificates` — trusted root store with enterprise CA concatenation support.
- **Security:**
  - Zero unneeded daemons (no Nginx, Apache), packet sniffers (`tcpdump`, `wireshark`), port scanners (`nmap`), compilers, or scripting runtimes.
  - Zero known vulnerabilities (0 CVE according to Trivy / Grype audits).
  - Runs under dedicated unprivileged user `kncc-agent` (UID 10001) with hardened Kubernetes SecurityContext (`Drop: [ALL]`, `Add: [NET_RAW]`, `allowPrivilegeEscalation: true` for ICMP raw socket operation in HostNetwork mode).

**Building custom `kncc-agent` image:**
```bash
docker build -f Dockerfile.agent -t ghcr.io/AleksandrSumkin/kncc-agent:latest .
docker push ghcr.io/AleksandrSumkin/kncc-agent:latest
```

#### 2. Supported Public and Alternative Images
`kncc` is architecturally compatible with a wide range of standard network troubleshooting images:
- **`wbitt/network-multitool:alpine-extra` (Public default fallback):** General-purpose diagnostic image (~120 MB) containing curl, nc, ping, nginx, tcpdump, nmap. Pulled directly from Docker Hub without requiring a local registry.
- **`wbitt/network-multitool:minimal`:** Slimmed down multitool variant (~40 MB).
- **`nicolaka/netshoot:latest`:** Feature-rich Swiss Army knife (~400 MB) with advanced utilities (`iperf3`, `mtr`, `tshark`, `socat`). Recommended for interactive debugging via `kubectl exec`.
- **`curlimages/curl:latest`:** Minimalist official curl image (~15 MB). Supports *only* HTTP/HTTPS checks (TCP, UDP, and ICMP checks will fail due to missing `nc` and `ping`).

#### 3. Image Compatibility Matrix

| Container Image | Type | Size | TCP | UDP | HTTP | HTTPS + X.509 | ICMP Ping | CVE (Trivy) | Recommendation |
|---|---|---|---|---|---|---|---|---|---|
| **`kncc-agent` (custom)** | **Native** | **~15 MB** | **✅** | **✅** | **✅** | **✅** | **✅** | **0** | **Recommended (Production & Preview).** Instant cold-start, Zero Attack Surface. |
| `wbitt/network-multitool:alpine-extra` | Public | ~120 MB | ✅ | ✅ | ✅ | ✅ | ✅ | Low | Public default fallback. No local registry needed. |
| `wbitt/network-multitool:minimal` | Public | ~40 MB | ✅ | ✅ | ✅ | ⚠️ | ✅ | Low | Lightweight multitool with reduced tooling. |
| `nicolaka/netshoot:latest` | Public | ~400 MB | ✅ | ✅ | ✅ | ✅ | ✅ | Medium | In-depth interactive debugging (`kubectl exec`). |
| `curlimages/curl:latest` | Public | ~15 MB | ❌ | ❌ | ✅ | ⚠️ (no X.509 dates) | ❌ | 0 | Specialized HTTP/HTTPS-only usage. |

#### 4. System Contract for Custom Images
If organizational policy mandates proprietary base images (e.g. RHEL UBI, Debian slim, Chainguard), custom images must meet the following minimum contract:
1. **Shell:** POSIX-compliant `/bin/sh`.
2. **DNS:** `getent hosts <host>` or `nslookup <host>`.
3. **TCP:** `nc` supporting `-z` and `-w <sec>`.
4. **UDP:** `nc` supporting `-u -z -w <sec>`.
5. **HTTP/HTTPS:** `curl` supporting `-s -S -o /dev/null -w "%{http_code}:%{time_total}:%{remote_ip}" --connect-timeout <sec> -m <sec>`.
6. **HTTPS X.509:** `openssl s_client -connect <host>:<port> -servername <host>` and `openssl x509 -dateopt iso_8601 -noout -enddate / -subject / -issuer`.
7. **ICMP:** `ping` supporting `-c <count> -W <sec>` (requires capability `NET_RAW`).
8. **Filesystem:** Write access to `/dev/termination-log` or `/tmp/termination-log` (chmod 1777).

---

## 4. Security & Abuse Prevention

- **Anti-SSRF & Blacklisting:** Built-in blocking of sensitive metadata IP ranges (`169.254.169.254/32`, `metadata.google.internal`, `127.0.0.0/8`, `::1/128`).
- **Anti-DNS Rebinding:** Domain names are dynamically resolved, and resolved IPs are verified against CIDR blacklists prior to socket creation.
- **Rate Limiting:** Request throttling per authenticated user (default 30 requests per minute).
- **Role Model (kncc RBAC):**
  - `Admin`: System configuration, cluster policies, user directory, and all reports.
  - `User`: Creates checks, inspects personal history, views/reruns shared reports via direct links.
- **Authentication Providers:** Simultaneous support for local SQLite credentials (dev/debug), LDAP / Active Directory, and OpenID Connect SSO (Keycloak, Dex, Okta, Azure AD).

---

## 5. Quick Start

### Local Demo Mode (Mock Mode)

`kncc` can run locally without connecting to a live Kubernetes cluster. In this mode, the server simulates 4 cluster nodes with pre-seeded demo reports:

```bash
# Clone repository
git clone https://github.com/AleksandrSumkin/kncc.git
cd kncc

# Build and run server in mock mode
go run ./cmd/kncc -mock
```

Open your browser at: **`http://localhost:8080`**
- **Default Administrator:** Username `admin`, password `admin` (additional users can be created in `/admin` or provisioned via LDAP/OIDC).

---

## 6. Kubernetes Deployment via Helm

The production Helm chart is located in `deploy/helm/kncc`.

### 6.1. Verifying Release Integrity and Digital Signatures

Before deploying release artifacts to production clusters, verify digital signatures and checksums:

```bash
# 1. Download release public key, checksum manifest, and signature from Forgejo Release page
# 2. Verify digital signature of checksum manifest
openssl dgst -sha256 -verify kncc-release.pub -signature kncc-1.16.0-checksums.txt.sig kncc-1.16.0-checksums.txt
# Expected output: Verified OK

# 3. Verify artifact checksums (Helm chart kncc-1.16.0.tgz, manifests, SBOM)
sha256sum -c kncc-1.16.0-checksums.txt
# Expected output: kncc-1.16.0.tgz: OK
```

### 6.2. Standard Installation

By default, `kncc` auto-generates a secure administrator password on first startup (printed to pod logs: `kubectl logs -n kncc -l app.kubernetes.io/name=kncc`):

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace
```

Alternatively, set an explicit initial administrator password:

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set auth.adminInitialPassword="MySecurePassword123"
```

### 6.3. Enabling Multi-Namespace Diagnostics (`multiNamespace: true`)

By default, `kncc` creates probe pods strictly inside its own namespace (`kncc`). To allow developers to evaluate network policies from within application namespaces, enable extended RBAC:

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set rbac.multiNamespace=true \
  --set auth.adminInitialPassword="MySecurePassword123"
```

### 6.4. Enabling Namespace Discovery Dropdown (`namespaceListEnabled: true`)

To provide users with an interactive namespace dropdown in the UI:

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set rbac.multiNamespace=true \
  --set k8s.namespaceListEnabled=true \
  --set auth.adminInitialPassword="MySecurePassword123"
```

*Security note:* Cluster-wide namespace permissions (`namespaces: ["get", "list", "watch"]`) are included in `ClusterRole` only when `k8s.namespaceListEnabled: true`.

### 6.5. Ingress Publishing

To expose the web UI and API through an Ingress controller (e.g. Traefik in k3s):

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set rbac.multiNamespace=true \
  --set k8s.namespaceListEnabled=true \
  --set ingress.enabled=true \
  --set ingress.className="traefik" \
  --set ingress.hosts[0].host="kncc.example.com" \
  --set ingress.hosts[0].paths[0].path="/" \
  --set ingress.hosts[0].paths[0].pathType="Prefix"
```

Access the interface at: `http://kncc.example.com`.

### 6.6. Gateway API Publishing (HTTPRoute)

To expose the service using modern Kubernetes Gateway API (`gateway.networking.k8s.io/v1`):

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set gateway.enabled=true \
  --set gateway.parentRefs[0].name="default-gateway" \
  --set gateway.hostnames[0]="kncc.example.com"
```

Optionally provision a dedicated `Gateway` resource directly via the chart:

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set gateway.enabled=true \
  --set gateway.createGateway=true \
  --set gateway.gatewayClassName="traefik" \
  --set gateway.hostnames[0]="kncc.example.com"
```

### 6.7. UI Theming Customization

Administrators can customize background and accent brand colors:

```bash
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --set theme.appName="Corporate Network Diagnostics" \
  --set theme.primaryColor="#ffffff" \
  --set theme.accentColor="#2563eb" # Blue accent instead of red
```

### 6.8. ServiceAccount Kubernetes API RBAC Permission Audit

For transparent cluster governance, `kncc` audits effective RBAC permissions:
1. **Startup Logging (Optional via `rbac.printAccessList` / `k8s.print_access_list`):** When enabled (`printAccessList: true`), the service evaluates effective ServiceAccount rules on boot (via `SelfSubjectRulesReview` or role binding traversal) and logs a formatted ASCII table matching the `rbac-tool policy-rules` standard (`TYPE | SUBJECT | VERBS | NAMESPACE | API GROUP | KIND | NAMES | NONRESOURCEURI | ORIGINATED FROM`). By default (`false`), startup logs remain clean and concise.
2. **Admin Dashboard (`/#/admin`):** The "Kubernetes SA Permissions (RBAC)" tab presents an interactive table with resource filters, verb badges, scope indicators, and copy actions (ASCII or JSON).
3. **Endpoint Security:** The `GET /api/admin/rbac` endpoint is restricted strictly to users with the `admin` role.

### 6.9. User Activity and Query Statistics

The administrative dashboard (`/#/admin`, "Query Statistics" tab) provides usage metrics:
1. **Time Breakdown:** Track checks executed across 24 hours, 7 days, 30 days, 365 days, and all-time.
2. **KPI Summary Cards:** Service load metrics at a glance.
3. **User Table with Sorting:** User avatar, role, auth provider (local/ldap/oidc), and last active timestamp.
4. **Summary Footer:** Automated totals across all active user accounts.
5. **TSV Export:** One-click clipboard export for spreadsheet analysis.
6. **Persistent Retention:** Activity logged in `user_request_logs` independent of report TTL.

### 6.10. Existing Secret for Administrator Password (External Secrets / Vault)

When managing secrets via HashiCorp Vault, External Secrets Operator (ESO), Sealed Secrets, or SOPS:

```bash
# Example pre-created secret:
kubectl create secret generic kncc-admin-vault \
  --namespace kncc \
  --from-literal=admin-password="SuperSecurePasswordFromVault"

# Helm installation referencing existing secret:
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set auth.existingSecret="kncc-admin-vault" \
  --set auth.existingSecretPasswordKey="admin-password"
```

Setting `auth.existingSecret` disables creation of the internal `kncc-secrets` resource for administrator credentials.

### 6.11. Quick Add Target Presets

On the New Check page, Quick Add chips dynamically assist users:
1. **Initial Presets:** New users see configured default targets (1 to 3 services).
2. **Adaptive Learning:** Recent user targets automatically populate suggestions.
3. **Top-3 Services:** Experienced users see their 3 most frequently checked endpoints.

Configure defaults in Helm `values.yaml`:
```yaml
quickAdd:
  presets:
    - name: "K8s API"
      protocol: "tcp"
      host: "10.96.0.1"
      port: 443
    - name: "Google DNS"
      protocol: "icmp"
      host: "8.8.8.8"
```

### 6.12. Kubernetes Health Probes (Startup, Liveness, Readiness)

For reliable cluster orchestration, `kncc` exposes standard health probes:
1. **Startup Probe (`GET /api/startupz`)**: Evaluated on startup prior to liveness checks. Grants sufficient time for volume mounting, SQLite schema migrations, and admin account setup.
2. **Liveness Probe (`GET /api/livez`)**: Monitors HTTP server responsiveness and Go runtime health without deadlocks, decoupled from storage status to prevent cascading restarts.
3. **Readiness Probe (`GET /api/readyz`)**: Actively probes SQLite database availability (`PingContext`). Returns `503 Service Unavailable` (`{"status": "degraded"}`) if database is unreachable, removing pod endpoints without process termination.
4. **Healthz (`GET /api/healthz`)**: Transparently proxies to `Readyz` for backward compatibility with legacy monitoring systems.

### 6.13. Corporate Logo and Favicon Branding

#### Default Logo
Ships with a high-resolution vector emblem centered around **shipbuilding and marine engineering** in blue/cyan tones (research vessel silhouette, hydrodynamics, and radar navigation mesh).

#### Custom Logo via Helm (Base64)
```bash
# Convert to Base64 (Linux/macOS):
base64 -w 0 company_logo.svg > logo.b64

# Deploy via Helm:
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --set-file logo.data=logo.b64 \
  --set logo.mimeType="image/svg+xml"
```

#### Custom Logo via Admin Dashboard
Navigate to **Admin** -> **App Settings** -> **Corporate Branding & Logo**, drag and drop a new logo (SVG, PNG, JPEG, WebP up to 1 MB), and click **Upload & Apply Logo**.

#### Custom Favicon
- **Default Fallback:** Favicon is automatically generated as a thumbnail of the active corporate logo.
- **Custom Upload:** Upload custom ICO, SVG, PNG, WebP icons (up to 512 KB) via Helm or the Admin dashboard with instant browser tab preview.

### 6.14. Administrator Password Delivery via emptyDir Volume (`medium: Memory`)

To comply with strict security baselines (CIS Kubernetes Benchmark, DevSecOps), delivery of the initial admin password via an isolated RAM tmpfs volume is supported:
- **No `/proc` exposure:** Avoids leaking credentials in `/proc/$PID/environ`.
- **In-memory tmpfs (`medium: Memory`):** Password resides purely in RAM, never flushed to node disks.
- **No leak in dumps/logs:** Excluded from crash dumps, container engine logs, and `kubectl describe pod` outputs.

Configure in Helm `values.yaml`:
```yaml
auth:
  deliveryMode: "volume"
  volume:
    medium: "Memory"
    sizeLimit: "64Mi"
    mountPath: "/etc/kncc-auth"
    fileName: "admin-password"
    populateFromSecret: true
```

### 6.15. LDAP / Active Directory Access Methods

1. **Search-and-Bind (Default):** Pre-queries user DN via service account (`auth.ldap.bindDN` / `auth.ldap.bindPassword`), followed by user credential bind.
2. **Existing Secret (`existingSecret`):** Securely injects LDAP service credentials from Kubernetes Secret (`auth.ldap.existingSecret`).
3. **Direct Bind / User Bind Pattern:** Binds directly under user credentials using a DN template (`auth.ldap.userDNTemplate: "uid=%s,ou=users,dc=example,dc=com"`), requiring no service account.
4. **Anonymous Search (RFC 4513):** Executes unauthenticated directory queries (`auth.ldap.anonymousSearch: true`).

### 6.16. Application Logging Level Configuration (`server.logLevel`)

To control log verbosity across all backend modules (including LDAP/OIDC authentication connectors, K8s probe runners, garbage collection, and HTTP API access):

```bash
# Enable verbose debug logging for troubleshooting
helm upgrade --install kncc ./deploy/helm/kncc \
  --namespace kncc \
  --create-namespace \
  --set server.logLevel=debug
```

Supported values: `debug`, `info` (default), `warn`, `error`.

---

## 7. Helm Values Reference

> [!TIP]
> All chart parameters are thoroughly documented inside [`deploy/helm/kncc/values.yaml`](deploy/helm/kncc/values.yaml) with explanations of runtime impacts and security recommendations (Zero-Docs Onboarding). Russian reference comments are preserved in [`deploy/helm/kncc/values_ru.yaml`](deploy/helm/kncc/values_ru.yaml).

| Parameter | Description | Default |
| :--- | :--- | :--- |
| `replicaCount` | kncc server pod replicas (Single-Writer SQLite constraint) | `1` |
| `image.repository` | Container image repository | `ghcr.io/aleksandrsumkin/kncc` |
| `image.tag` | Image tag override (defaults to `appVersion`) | `""` |
| `image.pullPolicy` | Image pull policy (`Always`, `IfNotPresent`) | `IfNotPresent` |
| `server.port` | HTTP API and UI port inside container | `8080` |
| `server.logLevel` | Application logging level (`debug`, `info`, `warn`, `error`) | `"info"` |
| `server.tokenTTL` | JWT session lifetime | `24h` |
| `server.tls.enabled` | Enable HTTPS/TLS server on dedicated port | `true` |
| `server.tls.port` | Dedicated HTTPS port inside container | `8443` |
| `server.tls.autoGenerate` | Auto-generate self-signed ECDSA P-256 certificate | `true` |
| `server.tls.certFile` | Path to certificate file inside container | `""` |
| `server.tls.keyFile` | Path to private key file inside container | `""` |
| `server.tls.certPem` | Inline PEM certificate string | `""` |
| `server.tls.keyPem` | Inline PEM private key string | `""` |
| `server.tls.minVersion` | Minimum TLS version (`1.2` or `1.3`) | `"1.2"` |
| `server.tls.existingSecret` | Existing Kubernetes Secret containing `tls.crt` and `tls.key` | `""` |
| `server.tls.mountPath` | Secret mount path inside container | `/etc/kncc-tls` |
| `server.tls.certManager.enabled` | Use cert-manager for automated server TLS certificates | `false` |
| `server.openapi.enabled` | Enable OpenAPI 3.x and Swagger UI (`/api/docs`) | `true` |
| `server.openapi.public` | Anonymous API documentation access (for DAST scanners) | `false` |
| `service.tlsPort` | External HTTPS service port (mapped to `server.tls.port`) | `443` |
| `k8s.probeImage` | Image for ephemeral diagnostic probe pods | `ghcr.io/aleksandrsumkin/kncc-agent:latest` |
| `k8s.probeTimeoutSeconds` | Individual probe network operation timeout | `30` |
| `k8s.probeActiveDeadlineSeconds` | Probe pod hard execution deadline | `60` |
| `k8s.probeImagePullPolicy` | Container image pull policy for probe pods (`Always`, `IfNotPresent`, `Never`) | `""` |
| `k8s.probeImagePullSecrets` | Registry credentials for private probe container images | `[]` |
| `k8s.probeExtraLabels` | Additional labels attached to probe pods | `{}` |
| `k8s.probeExtraAnnotations`| Additional annotations attached to probe pods (e.g. Service Mesh sidecar exclusion) | `{}` |
| `k8s.probePriorityClassName`| PriorityClass name assigned to probe pods | `""` |
| `k8s.probeResources` | Custom CPU and memory requests and limits for probe pods | `{}` |
| `k8s.probePodSecurityContext` | PodSecurityContext configuration for probe pods | `{}` |
| `k8s.probeSecurityContext` | Container SecurityContext configuration for probe pods | `{}` |
| `k8s.gcInterval` | Garbage collection sweep interval | `2m` |
| `k8s.gcStaleThreshold` | Minimum pod age threshold before GC removal | `3m` |
| `k8s.namespaceListEnabled` | Interactive namespace dropdown in UI | `true` |
| `k8s.namespaceExcludeList` | Namespaces excluded from UI and API execution | `["kube-system", "kube-public", "kube-node-lease"]` |
| `k8s.namespaceExcludeSelector` | LabelSelector for dynamic namespace exclusion | `"kncc.io/exclude=true"` |
| `rbac.create` | Provision ServiceAccount, Roles, and RoleBindings | `true` |
| `rbac.multiNamespace` | Permit scheduling probes across any namespace (ClusterRole) | `true` |
| `rbac.printAccessList` | Print effective ServiceAccount RBAC permissions table to startup logs | `false` |
| `security.rateLimitPerMinute` | Maximum allowed check requests per minute per user | `30` |
| `security.blacklistCIDRs` | Prohibited IP subnets (Anti-SSRF) | `["169.254.169.254/32", "127.0.0.0/8", "::1/128"]` |
| `security.blacklistDomains` | Prohibited domain patterns | `["metadata.google.internal", "instance-data"]` |
| `security.whitelistCIDRs` | Allowed IP subnets (Strict Allowlist mode) | `[]` |
| `storage.reportTTL` | Retention duration / time-to-live for diagnostic reports | `72h` |
| `persistence.enabled` | Use PVC for SQLite persistence across pod restarts | `false` |
| `persistence.size` | PVC capacity | `2Gi` |
| `auth.adminUsername` | Initial administrator username | `admin` |
| `auth.adminInitialPassword` | Initial admin password (auto-generated if empty) | `""` |
| `auth.existingSecret` | Existing Kubernetes Secret containing admin password | `""` |
| `auth.existingSecretPasswordKey` | Secret key for admin password | `ADMIN_INITIAL_PASSWORD` |
| `auth.deliveryMode` | Password delivery mechanism (`volume` or `env`) | `volume` |
| `auth.volume.medium` | Volume medium (`Memory` for RAM tmpfs or `""` for disk) | `Memory` |
| `auth.localEnabled` | Enable local SQLite username/password authentication | `true` |
| `auth.ldapEnabled` | Enable LDAP / Active Directory authentication | `false` |
| `auth.ldap.existingSecret` | Existing Secret containing LDAP credentials | `""` |
| `auth.ldap.userDNTemplate` | Direct Bind DN template (e.g. `uid=%s,ou=users,dc=example,dc=com`) | `""` |
| `auth.ldap.anonymousSearch` | Anonymous directory search mode (RFC 4513) | `false` |
| `auth.ldap.adminGroup` | LDAP group dynamically mapped to `admin` role | `""` |
| `auth.oidcEnabled` | Enable OpenID Connect SSO | `false` |
| `auth.oidc.issuerURL` | OIDC Identity Provider Discovery URL | `"https://auth.corp.example.com/realms/corp"` |
| `auth.oidc.clientID` | OIDC Client ID | `"kncc"` |
| `auth.oidc.existingSecret` | Existing Secret containing OIDC client credentials | `""` |
| `caCerts.enabled` | Enable mounting custom enterprise CA certificates | `false` |
| `caCerts.pem` | Inline PEM corporate CA bundle | `""` |
| `theme.appName` | Application title in UI header | `Kubernetes Network Connectivity Checker` |
| `theme.primaryColor` | Base background color | `#ffffff` |
| `theme.accentColor` | Accent brand color for buttons and badges | `#dc2626` |
| `theme.defaultTheme` | Default theme for new sessions (`light` or `dark`) | `light` |
| `theme.defaultLanguage` | Default UI language (`en` or `ru`) | `en` |
| `logo.enabled` | Enable corporate logo ConfigMap | `true` |
| `favicon.enabled` | Enable custom favicon ConfigMap | `true` |
| `quickAdd.presets` | Quick-add diagnostic target presets | `[{name: "K8s API", ...}, {name: "Google DNS", ...}]` |
| `ingress.enabled` | Provision Ingress resource | `false` |
| `gateway.enabled` | Provision Gateway API HTTPRoute | `false` |
| `probes.startup.enabled` | Enable Startup Probe | `true` |
| `probes.liveness.enabled` | Enable Liveness Probe | `true` |
| `probes.readiness.enabled`| Enable Readiness Probe | `true` |

---

## 8. REST API Reference

| Method | Endpoint | Access | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/livez` | Public | Kubernetes Liveness Probe: Go runtime health |
| `GET` | `/api/readyz` | Public | Kubernetes Readiness Probe: Active SQLite database validation |
| `GET` | `/api/startupz` | Public | Kubernetes Startup Probe: Schema migrations and initialization check |
| `GET` | `/api/healthz` | Public | Backward compatibility alias for `/api/readyz` |
| `GET` | `/api/system/config` | Public | System configuration, theme parameters, and auth providers |
| `GET` | `/api/system/logo` | Public | Active corporate logo (SVG/PNG with ETag caching) |
| `GET` | `/api/system/favicon` | Public | Active favicon (custom or corporate logo thumbnail) |
| `GET` | `/favicon.ico` | Public | Standard browser favicon route |
| `GET` | `/api/auth/providers` | Public | Available authentication providers (local, ldap, oidc) |
| `POST` | `/api/auth/login` | Public | User authentication returning JWT token |
| `GET` | `/api/auth/oidc/login` | Public | Initiate OIDC / SSO authentication flow |
| `GET` | `/api/auth/oidc/callback`| Public | Handle OAuth2/OIDC redirect, issue JWT, and transition to UI |
| `POST` | `/api/auth/oidc/callback`| Public | Exchange OIDC authorization code for JWT token |
| `POST` | `/api/auth/logout` | Authenticated | Terminate session and clear cookies |
| `GET` | `/api/auth/me` | Authenticated | Profile and role of authenticated user |
| `GET` | `/api/k8s/namespaces` | Authenticated | List allowed cluster namespaces (403 if disabled) |
| `GET` | `/api/checks/quick-add`| Authenticated | Dynamic target suggestions for user |
| `POST` | `/api/checks` | Authenticated | Launch network diagnostic check across cluster nodes |
| `GET` | `/api/reports` | Authenticated | Paginated connectivity report list |
| `GET` | `/api/reports/{id}` | Authenticated | Diagnostic report with node reachability matrix |
| `POST` | `/api/reports/{id}/rerun`| Authenticated | Rerun connectivity check from existing report |
| `GET` | `/api/admin/users` | Administrator | List registered user accounts |
| `POST` | `/api/admin/users` | Administrator | Create local user account |
| `GET` | `/api/admin/stats/users` | Administrator | Query execution statistics across users and timeframes |
| `GET` | `/api/admin/rbac` | Administrator | Effective ServiceAccount RBAC permissions on Kubernetes API |
| `POST` | `/api/admin/settings/logo` | Administrator | Upload custom corporate logo |
| `DELETE` | `/api/admin/settings/logo`| Administrator | Reset corporate logo to default |
| `POST` | `/api/admin/settings/favicon`| Administrator | Upload custom site favicon |
| `DELETE` | `/api/admin/settings/favicon`| Administrator | Reset favicon to corporate logo thumbnail |
| `GET` | `/api/openapi.json` | Configurable (`public: false`/`true`) | Machine-readable OpenAPI 3.x specification in JSON |
| `GET` | `/api/openapi.yaml` | Configurable (`public: false`/`true`) | Machine-readable OpenAPI 3.x specification in YAML |
| `GET` | `/api/docs` | Configurable (`public: false`/`true`) | Interactive Swagger UI documentation |

---

## 9. License

This project is licensed under the **Apache 2.0 License**.
