{{/*
Expand the name of the chart.
*/}}
{{- define "kncc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kncc.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kncc.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kncc.labels" -}}
helm.sh/chart: {{ include "kncc.chart" . }}
{{ include "kncc.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kncc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kncc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kncc.serviceAccountName" -}}
{{- if .Values.rbac.serviceAccount.create }}
{{- default (include "kncc.fullname" .) .Values.rbac.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.rbac.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the admin secret name
*/}}
{{- define "kncc.adminSecretName" -}}
{{- if .Values.auth.existingSecret }}
{{- .Values.auth.existingSecret }}
{{- else }}
{{- printf "%s-secrets" (include "kncc.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Get the admin secret key
*/}}
{{- define "kncc.adminSecretKey" -}}
{{- if .Values.auth.existingSecret }}
{{- default "ADMIN_INITIAL_PASSWORD" .Values.auth.existingSecretPasswordKey }}
{{- else }}
{{- "ADMIN_INITIAL_PASSWORD" }}
{{- end }}
{{- end }}

{{/*
Get the OIDC secret name
*/}}
{{- define "kncc.oidcSecretName" -}}
{{- if and .Values.auth.oidc .Values.auth.oidc.existingSecret }}
{{- .Values.auth.oidc.existingSecret }}
{{- else }}
{{- printf "%s-secrets" (include "kncc.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Get the OIDC client secret key
*/}}
{{- define "kncc.oidcClientSecretKey" -}}
{{- if and .Values.auth.oidc .Values.auth.oidc.existingSecret }}
{{- default "OIDC_CLIENT_SECRET" .Values.auth.oidc.existingSecretClientSecretKey }}
{{- else }}
{{- "OIDC_CLIENT_SECRET" }}
{{- end }}
{{- end }}

{{/*
Get the OIDC client ID key
*/}}
{{- define "kncc.oidcClientIDKey" -}}
{{- default "OIDC_CLIENT_ID" .Values.auth.oidc.existingSecretClientIDKey }}
{{- end }}

{{/*
Return true if an admin initial password is explicitly configured (via adminInitialPassword or existingSecret)
*/}}
{{- define "kncc.hasAdminPassword" -}}
{{- if or .Values.auth.existingSecret .Values.auth.adminInitialPassword -}}
true
{{- else -}}
false
{{- end -}}
{{- end }}

{{/*
Return true if kncc-secrets Secret resource should be created
*/}}
{{- define "kncc.createSecret" -}}
{{- if or (and (not .Values.auth.existingSecret) .Values.auth.adminInitialPassword) (and .Values.auth.ldapEnabled .Values.auth.ldap.bindPassword (not .Values.auth.ldap.existingSecret)) (and .Values.auth.oidcEnabled .Values.auth.oidc.clientSecret (not .Values.auth.oidc.existingSecret)) (and .Values.metrics (and .Values.metrics.enabled (and .Values.metrics.auth (and .Values.metrics.auth.enabled (and (or .Values.metrics.auth.password .Values.metrics.auth.token) (not .Values.metrics.auth.existingSecret)))))) }}
{{- true }}
{{- end }}
{{- end }}

{{/*
Determine the volume source type for CA certificates (secret, configMap, or none)
*/}}
{{- define "kncc.caVolumeType" -}}
{{- if and .Values.caCerts (and .Values.caCerts.certManager .Values.caCerts.certManager.enabled) -}}
secret
{{- else if and .Values.caCerts .Values.caCerts.existingSecret -}}
secret
{{- else if and .Values.caCerts .Values.caCerts.existingConfigMap -}}
configMap
{{- else if and .Values.caCerts .Values.caCerts.pem -}}
configMap
{{- else -}}
none
{{- end -}}
{{- end -}}

{{/*
Determine the name of the Secret or ConfigMap containing the CA certificates
*/}}
{{- define "kncc.caResourceName" -}}
{{- if and .Values.caCerts (and .Values.caCerts.certManager .Values.caCerts.certManager.enabled) -}}
{{- printf "%s-ca-cert" (include "kncc.fullname" .) -}}
{{- else if and .Values.caCerts .Values.caCerts.existingSecret -}}
{{- .Values.caCerts.existingSecret -}}
{{- else if and .Values.caCerts .Values.caCerts.existingConfigMap -}}
{{- .Values.caCerts.existingConfigMap -}}
{{- else if and .Values.caCerts .Values.caCerts.pem -}}
{{- printf "%s-custom-ca" (include "kncc.fullname" .) -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{/*
Determine whether a Kubernetes Secret is used for server TLS certificate
*/}}
{{- define "kncc.hasServerTlsSecret" -}}
{{- if and .Values.server.tls .Values.server.tls.enabled -}}
{{- if or .Values.server.tls.existingSecret (and .Values.server.tls.certManager .Values.server.tls.certManager.enabled) -}}
true
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Determine the secret name for the server TLS certificate
*/}}
{{- define "kncc.serverTlsSecretName" -}}
{{- if and .Values.server.tls .Values.server.tls.existingSecret -}}
{{- .Values.server.tls.existingSecret -}}
{{- else -}}
{{- printf "%s-server-tls" (include "kncc.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
Get the metrics secret name
*/}}
{{- define "kncc.metricsSecretName" -}}
{{- if and .Values.metrics (and .Values.metrics.auth .Values.metrics.auth.existingSecret) }}
{{- .Values.metrics.auth.existingSecret }}
{{- else }}
{{- printf "%s-secrets" (include "kncc.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Get the metrics username key
*/}}
{{- define "kncc.metricsUsernameKey" -}}
{{- if and .Values.metrics (and .Values.metrics.auth .Values.metrics.auth.existingSecretUsernameKey) }}
{{- .Values.metrics.auth.existingSecretUsernameKey }}
{{- else }}
{{- "METRICS_AUTH_USERNAME" }}
{{- end }}
{{- end }}

{{/*
Get the metrics password key
*/}}
{{- define "kncc.metricsPasswordKey" -}}
{{- if and .Values.metrics (and .Values.metrics.auth .Values.metrics.auth.existingSecretPasswordKey) }}
{{- .Values.metrics.auth.existingSecretPasswordKey }}
{{- else }}
{{- "METRICS_AUTH_PASSWORD" }}
{{- end }}
{{- end }}

{{/*
Get the metrics token key
*/}}
{{- define "kncc.metricsTokenKey" -}}
{{- if and .Values.metrics (and .Values.metrics.auth .Values.metrics.auth.existingSecretTokenKey) }}
{{- .Values.metrics.auth.existingSecretTokenKey }}
{{- else }}
{{- "METRICS_AUTH_TOKEN" }}
{{- end }}
{{- end }}

{{/*
Construct the server container image reference (repository, tag or digest)
*/}}
{{- define "kncc.image" -}}
{{- if .Values.image.digest -}}
    {{- if hasPrefix "@" .Values.image.digest -}}
        {{- printf "%s%s" .Values.image.repository .Values.image.digest -}}
    {{- else -}}
        {{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
    {{- end -}}
{{- else -}}
    {{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
{{- end -}}

{{/*
Construct the probe/agent container image reference
*/}}
{{- define "kncc.probeImage" -}}
{{- $digest := "" -}}
{{- if and .Values.k8s .Values.k8s.probeImageDigest -}}
    {{- $digest = .Values.k8s.probeImageDigest -}}
{{- else if and .Values.agent (and .Values.agent.image .Values.agent.image.digest) -}}
    {{- $digest = .Values.agent.image.digest -}}
{{- end -}}
{{- $baseImage := .Values.k8s.probeImage -}}
{{- if and .Values.agent (and .Values.agent.image .Values.agent.image.repository) -}}
    {{- if .Values.agent.image.tag -}}
        {{- $baseImage = printf "%s:%s" .Values.agent.image.repository .Values.agent.image.tag -}}
    {{- else -}}
        {{- $baseImage = .Values.agent.image.repository -}}
    {{- end -}}
{{- end -}}
{{- if $digest -}}
    {{- $cleanBase := regexReplaceAll "@.*$" $baseImage "" -}}
    {{- $cleanBase = regexReplaceAll ":[^/:]+$" $cleanBase "" -}}
    {{- if hasPrefix "@" $digest -}}
        {{- printf "%s%s" $cleanBase $digest -}}
    {{- else -}}
        {{- printf "%s@%s" $cleanBase $digest -}}
    {{- end -}}
{{- else -}}
    {{- $baseImage -}}
{{- end -}}
{{- end -}}


