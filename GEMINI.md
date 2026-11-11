# Kubernetes Network Connectivity Checker (kncc)

## 1. Problem Statement & Purpose
In enterprise environments and multi-tenant Kubernetes clusters with strict network segmentation (Firewalls, CNI NetworkPolicies, Calico/Cilium), regular developers and support engineers lack direct node access (`node access`) and are prohibited from running `kubectl exec` into containers.
During application deployment, scenarios frequently arise where an external service (database, corporate API, external message broker) is unreachable from specific cluster nodes or namespaces. Applications crash on startup with network connection errors, delaying troubleshooting and posing business risks.

**kncc** is a lightweight, open-source, autonomous network connectivity diagnostic tool for Kubernetes. Via an intuitive web UI and REST API, it allows parallel validation of external service reachability from every cluster node, both at the node network layer (host network) and from inside the pod network (accounting for NetworkPolicies).

---

## 2. Architecture & Key Components

### 2.1. Backend (Go)
- **Execution Model:** On-demand Pods. When a check is requested, the Backend leverages the Kubernetes API to create short-lived probe pods on each cluster node concurrently (utilizing `nodeName` and system `Tolerations` to bypass master/control-plane and custom node taints).
- **Probe Image:** Utilizes a lightweight network diagnostic image (defaulting to `ghcr.io/aleksandrsumkin/kncc-agent` or `wbitt/network-multitool`, configured in `values.yaml`).
- **Results Collection:** The probe writes structured diagnostic JSON (status, latency, error code, resolved IP) to `/dev/termination-log` (`terminationMessagePolicy: FallbackToLogsOnError`). The Backend extracts results directly from the completed pod status with zero network overhead.
- **Pod Lifecycle & Cleanup:**
  - Each probe has a hard execution deadline (`activeDeadlineSeconds: 60`).
  - An internal background Garbage Collector (GC) in `kncc` periodically discovers and purges any orphaned or completed pods bearing the `app.kubernetes.io/managed-by: kncc` label.
- **Supported Protocols:**
  - **TCP:** Connection establishment verification (SYN/ACK handshake).
  - **UDP:** Datagram transmission and response handling / verifying absence of ICMP Port Unreachable.
  - **HTTP / HTTPS:** HTTP status code validation (2xx/3xx), TLS certificate validation, or optional skip-verify.
  - **ICMP (ping):** Round-trip time (RTT) and packet loss measurement.
- **Network Check Modes:**
  - `Host Network`: Schedules probe pods with `hostNetwork: true` to test physical node routing, gateways, and corporate firewalls.
  - `Pod Network`: Schedules probe pods inside a user-specified `Namespace` to emulate target application network context and evaluate `NetworkPolicy` restrictions.
  - Namespaced execution is controlled via configuration / Helm values (enabling extended RBAC).
- **Security & Abuse Prevention (Anti-SSRF & Rate Limiting):**
  - Configurable IP/CIDR/domain blacklists and whitelists (e.g. blocking `169.254.169.254/32`, internal k8s API).
  - Built-in Rate Limiter (default 30 requests per minute per user, configurable in Helm values).
- **Authentication & Authorization (Auth & RBAC):**
  - Simultaneous support for three provider types:
    1. Local users (for dev/debug environments).
    2. LDAP / Active Directory.
    3. SSO / OIDC (Keycloak, Dex, Okta, Azure AD).
  - Role model:
    - `Admin`: Access to system configuration, user management, and all cluster reports.
    - `User`: Capability to initiate checks and inspect personal check history.
  - **Report Sharing:** Each report has a unique UUID. Any authorized user with a direct link can view a colleague's report and trigger "Rerun" (initiating a new check on behalf of the current user).
- **Data Persistence:**
  - Embedded SQLite database mounted on a volume (`emptyDir` by default, optionally PersistentVolumeClaim in Helm).
  - Automated report retention rotation and TTL cleanup.

### 2.2. Frontend (React + TypeScript + Tailwind CSS)
- **Build & Delivery:** Static SPA bundled with Vite, embedded into the Go executable via `embed.FS`.
- **Theming:** Clean interface styled around 2 configurable core colors: base background (default white) and brand accent (default red). Colors are customized by administrators via environment variables (`THEME_PRIMARY_COLOR`, `THEME_ACCENT_COLOR`) and applied dynamically via CSS variables.
- **Pages & Features:**
  - Login screen (with provider selection when multiple are active) and logout action.
  - New check form: multi-target input (`protocol`, `host`, `port`), mode selector (`Host Network` or `Pod Network` with namespace selection), initiated via concise **"Run Check"** button.
  - Interactive report dashboard:
    - Two visualization perspectives: **By Target** (aggregated node reachability, status badges `ALL NODES REACHABLE` / `PARTIALLY REACHABLE` / `UNREACHABLE`, interactive node chips) and **By Node** (classic node-centric breakdown).
    - Collapsible target cards, quick **Expand all** / **Collapse all** actions, status filters (`All`, `Issues Only`, `All Reachable`), search by host and node name.
    - **"Back to Edit"** button (pre-populating the check creation form with parameters from the current report for quick fixes).
  - "Share Link" and "Rerun Check" capabilities.
  - Administrator panel (for users with Admin role).

### 2.3. Helm Chart (`deploy/helm/kncc`)
- Production-ready Helm chart:
  - `Deployment` for kncc server.
  - `Service` and `Ingress` / `HTTPRoute` (Gateway API).
  - `ServiceAccount`, `ClusterRole`, `ClusterRoleBinding` (with `rbac.multiNamespace: true|false` toggle).
  - `ConfigMap` and `Secret` for auth, rate limiting, blacklists, and theme colors.
  - `PersistentVolumeClaim` / `emptyDir` for SQLite database.

---

## 3. Repository Structure

```text
kncc/
├── cmd/
│   └── kncc/                   # Primary backend entrypoint
│       └── main.go
├── internal/
│   ├── api/                    # HTTP REST API handlers and routing
│   ├── auth/                   # Authentication providers (Local, LDAP, OIDC)
│   ├── certutil/               # TLS & certificate generation utilities
│   ├── config/                 # Configuration parsing and validation (env, YAML)
│   ├── k8s/                    # Kubernetes client, probe launcher, GC
│   ├── model/                  # Domain models (CheckRequest, Report, NodeResult)
│   ├── ratelimit/              # Rate limiting middleware
│   ├── security/               # Anti-SSRF CIDR/domain validation
│   └── store/                  # SQLite store (migrations, CRUD operations)
├── web/                        # React + Vite + Tailwind CSS frontend source
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── deploy/
│   └── helm/
│       └── kncc/               # Helm chart for Kubernetes deployment
│           ├── Chart.yaml
│           ├── values.yaml     # English documentation comments
│           ├── values_ru.yaml  # Russian documentation comments
│           └── templates/
├── docs/                       # Architectural and deployment documentation
├── Dockerfile                  # Multi-stage container build
├── go.mod
├── go.sum
└── README.md
```

---

## 4. Development Guidelines & Conventions
1. **Clean Architecture & Modularity:** Go code follows standard packaging conventions (`internal/...`). Packages must avoid circular dependencies.
2. **Kubernetes Security:** Probe pods must enforce least privilege (`readOnlyRootFilesystem`, `drop capabilities` except those necessary for ICMP).
3. **Error Reporting:** All Kubernetes API and network probe errors must produce actionable messages in diagnostic reports (e.g., *"DNS resolution failed"*, *"Connection timed out (SYN sent, no response - check Firewall/SecurityGroup)"*, *"Connection refused (RST received - port closed)"*).
4. **Testing:** Core business logic (CIDR validation, termination-log parsing, pod manifest assembly) must be covered with Go unit tests.
5. **Frontend:** Avoid heavy, unnecessary dependencies. Utilize Tailwind CSS and Lucide icons for a responsive, lightweight UI.
