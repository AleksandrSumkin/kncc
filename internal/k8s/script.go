package k8s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"kncc/internal/model"
)

// TargetScriptData holds data for generating probe commands.
type TargetScriptData struct {
	Index          int
	Protocol       string
	Host           string
	Port           int
	Path           string
	InsecureTLS    bool
	TimeoutSeconds int
	TargetJSON     string
}

const probeScriptTemplate = `#!/bin/sh
set +e
LOG_FILE="/dev/termination-log"
if [ ! -w "/dev/termination-log" ]; then
  LOG_FILE="/tmp/termination-log"
fi

CA_BUNDLE="/etc/ssl/certs/ca-certificates.crt"
{{if .CustomCAPEM}}
# Install custom corporate CA certificates (chain of Root + Intermediate)
mkdir -p /tmp/kncc-certs
cat << 'EOF_KNCC_CA' > /tmp/kncc-certs/custom-ca.pem
{{.CustomCAPEM}}
EOF_KNCC_CA
if [ -f "/etc/ssl/certs/ca-certificates.crt" ]; then
  cat /etc/ssl/certs/ca-certificates.crt /tmp/kncc-certs/custom-ca.pem > /tmp/kncc-certs/ca-bundle.crt
else
  cp /tmp/kncc-certs/custom-ca.pem /tmp/kncc-certs/ca-bundle.crt
fi
CA_BUNDLE="/tmp/kncc-certs/ca-bundle.crt"
export SSL_CERT_FILE="$CA_BUNDLE"
export CURL_CA_BUNDLE="$CA_BUNDLE"
{{end}}

echo -n "[" > "$LOG_FILE"
FIRST=1

{{range .Targets}}
# --- Target {{.Index}}: {{.Protocol}} {{.Host}}:{{.Port}}{{.Path}} ---
TARGET_JSON='{{.TargetJSON}}'
HOST="{{.Host}}"
PORT="{{.Port}}"
PATH_URL="{{.Path}}"
TIMEOUT="{{.TimeoutSeconds}}"
SUCCESS=false
LATENCY_MS=0
RESOLVED_IP=""
HTTP_CODE=0
ERR_MSG=""
DETAILS=""
CERT_EXPIRY=""
CERT_SUBJECT=""
CERT_ISSUER=""

# Resolve IP
RESOLVED_IP=$(getent hosts "$HOST" 2>/dev/null | awk '{print $1; exit}')
if [ -z "$RESOLVED_IP" ]; then
  # Fallback to nslookup
  RESOLVED_IP=$(nslookup "$HOST" 2>/dev/null | awk '/^Address: / {print $2; exit}')
fi

{{if eq .Protocol "tcp"}}
# TCP Check
START=$(date +%s%3N 2>/dev/null || date +%s)
NC_OUT=$(nc -z -w "$TIMEOUT" "$HOST" "$PORT" 2>&1)
EXIT_CODE=$?
END=$(date +%s%3N 2>/dev/null || date +%s)
if [ "$START" -gt 0 ] && [ "$END" -ge "$START" ]; then
  LATENCY_MS=$(( END - START ))
fi

if [ $EXIT_CODE -eq 0 ]; then
  SUCCESS=true
  DETAILS="TCP connection established (SYN/ACK received)"
else
  SUCCESS=false
  if [ -z "$RESOLVED_IP" ]; then
    ERR_MSG="DNS resolution failed for host $HOST"
  elif echo "$NC_OUT" | grep -qi "refused"; then
    ERR_MSG="Connection refused (RST received - target port $PORT is closed)"
  else
    ERR_MSG="Connection timed out (SYN sent, no response - check Firewall/SecurityGroup/NetworkPolicy)"
  fi
fi

{{else if eq .Protocol "udp"}}
# UDP Check
START=$(date +%s%3N 2>/dev/null || date +%s)
NC_OUT=$(nc -u -z -w "$TIMEOUT" "$HOST" "$PORT" 2>&1)
EXIT_CODE=$?
END=$(date +%s%3N 2>/dev/null || date +%s)
if [ "$START" -gt 0 ] && [ "$END" -ge "$START" ]; then
  LATENCY_MS=$(( END - START ))
fi

if [ $EXIT_CODE -eq 0 ]; then
  SUCCESS=true
  DETAILS="UDP datagram sent without ICMP Port Unreachable error"
else
  SUCCESS=false
  if [ -z "$RESOLVED_IP" ]; then
    ERR_MSG="DNS resolution failed for host $HOST"
  else
    ERR_MSG="UDP port unreachable or blocked: $NC_OUT"
  fi
fi

{{else if or (eq .Protocol "http") (eq .Protocol "https")}}
# HTTP/HTTPS Check
URL="{{.Protocol}}://{{.Host}}:{{.Port}}{{.Path}}"
{{if .InsecureTLS}}
INSECURE_FLAG="-k"
{{else}}
INSECURE_FLAG=""
{{end}}
{{if $.CustomCAPEM}}
CA_FLAG="--cacert $CA_BUNDLE"
{{else}}
CA_FLAG=""
{{end}}

CURL_OUT=$(curl -s -S -o /dev/null -w "%{http_code}:%{time_total}:%{remote_ip}" --connect-timeout "$TIMEOUT" -m "$TIMEOUT" $INSECURE_FLAG $CA_FLAG "$URL" 2>&1)
CURL_EC=$?

if [ $CURL_EC -eq 0 ]; then
  HTTP_CODE=$(echo "$CURL_OUT" | awk -F: '{print $1}')
  TIME_SEC=$(echo "$CURL_OUT" | awk -F: '{print $2}')
  CURL_IP=$(echo "$CURL_OUT" | awk -F: '{print $3}')
  if [ -n "$CURL_IP" ]; then
    RESOLVED_IP="$CURL_IP"
  fi
  # Convert seconds to ms
  LATENCY_MS=$(awk -v t="$TIME_SEC" 'BEGIN {printf "%.0f", t * 1000}' 2>/dev/null || echo 0)

  if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 400 ]; then
    SUCCESS=true
    DETAILS="HTTP request successful with status $HTTP_CODE"
  else
    SUCCESS=false
    ERR_MSG="HTTP request returned status $HTTP_CODE"
  fi
else
  SUCCESS=false
  if [ -z "$RESOLVED_IP" ]; then
    ERR_MSG="DNS resolution failed for host $HOST"
  elif echo "$CURL_OUT" | grep -qi "timed out"; then
    ERR_MSG="HTTP connection timed out (firewall/security policy block)"
  elif echo "$CURL_OUT" | grep -qi "refused"; then
    ERR_MSG="Connection refused by remote host"
  elif echo "$CURL_OUT" | grep -qi "certificate"; then
    ERR_MSG="TLS/SSL certificate validation error: $CURL_OUT"
  else
    ERR_MSG="HTTP request failed: $CURL_OUT"
  fi
fi

{{if eq .Protocol "https"}}
# Extract TLS Certificate Expiry and Metadata
{{if $.CustomCAPEM}}
OPENSSL_CA_FLAG="-CAfile $CA_BUNDLE"
{{else}}
OPENSSL_CA_FLAG=""
{{end}}
CERT_RAW=$(echo | timeout "$TIMEOUT" openssl s_client -connect "$HOST:$PORT" -servername "$HOST" $OPENSSL_CA_FLAG 2>/dev/null)
if [ -n "$CERT_RAW" ]; then
  CERT_EXPIRY=$(echo "$CERT_RAW" | openssl x509 -dateopt iso_8601 -noout -enddate 2>/dev/null | sed -e 's/^notAfter=//')
  CERT_SUBJECT=$(echo "$CERT_RAW" | openssl x509 -noout -subject 2>/dev/null | sed -e 's/^subject=//')
  CERT_ISSUER=$(echo "$CERT_RAW" | openssl x509 -noout -issuer 2>/dev/null | sed -e 's/^issuer=//')
fi
{{end}}

{{else if eq .Protocol "icmp"}}
# ICMP Ping Check
PING_OUT=$(ping -c 2 -W "$TIMEOUT" "$HOST" 2>&1)
PING_EC=$?

if [ $PING_EC -eq 0 ]; then
  SUCCESS=true
  # Parse avg RTT from ping output
  AVG_RTT=$(echo "$PING_OUT" | grep -o 'round-trip min/avg/max[^=]*= [0-9.]*/[0-9.]*/' | awk -F/ '{print $2}' 2>/dev/null)
  if [ -z "$AVG_RTT" ]; then
    AVG_RTT=$(echo "$PING_OUT" | grep -o 'rtt min/avg/max[^=]*= [0-9.]*/[0-9.]*/' | awk -F/ '{print $2}' 2>/dev/null)
  fi
  LATENCY_MS=$(awk -v r="$AVG_RTT" 'BEGIN {printf "%.0f", r}' 2>/dev/null || echo 1)
  DETAILS="ICMP Echo Reply received"
else
  SUCCESS=false
  if [ -z "$RESOLVED_IP" ]; then
    ERR_MSG="DNS resolution failed for host $HOST"
  else
    ERR_MSG="ICMP ping packet loss / destination unreachable"
  fi
fi
{{end}}

# Escape JSON strings
clean_json() {
  if [ -z "$1" ]; then
    echo -n ""
  else
    echo -n "$1" | tr '\n' ' ' | tr '"' "'" | tr '\\' '/'
  fi
}
ERR_CLEAN=$(clean_json "$ERR_MSG")
DET_CLEAN=$(clean_json "$DETAILS")
CERT_EXP_CLEAN=$(clean_json "$CERT_EXPIRY")
CERT_SUBJ_CLEAN=$(clean_json "$CERT_SUBJECT")
CERT_ISS_CLEAN=$(clean_json "$CERT_ISSUER")

if [ $FIRST -eq 0 ]; then
  echo -n "," >> "$LOG_FILE"
fi
FIRST=0

cat <<EOF >> "$LOG_FILE"
{
  "target": $TARGET_JSON,
  "success": $SUCCESS,
  "latency_ms": $LATENCY_MS,
  "resolved_ip": "$RESOLVED_IP",
  "http_status_code": $HTTP_CODE,
  "error": "$ERR_CLEAN",
  "details": "$DET_CLEAN",
  "cert_expiry": "$CERT_EXP_CLEAN",
  "cert_subject": "$CERT_SUBJ_CLEAN",
  "cert_issuer": "$CERT_ISS_CLEAN"
}
EOF
{{end}}

echo -n "]" >> "$LOG_FILE"

# Also print to stdout for fallback container logs
cat "$LOG_FILE"
exit 0
`

var tmpl = template.Must(template.New("probe").Parse(probeScriptTemplate))

// BuildProbeScript generates the POSIX shell script executed by the probe container.
func BuildProbeScript(targets []model.TargetService) (string, error) {
	return BuildProbeScriptWithCA(targets, "")
}

// BuildProbeScriptWithCA generates the POSIX shell script with optional custom CA bundle.
func BuildProbeScriptWithCA(targets []model.TargetService, customCAPEM string) (string, error) {
	var targetData []TargetScriptData
	for i, t := range targets {
		tj, err := json.Marshal(t)
		if err != nil {
			return "", fmt.Errorf("marshaling target %d: %w", i, err)
		}
		targetData = append(targetData, TargetScriptData{
			Index:          i + 1,
			Protocol:       string(t.Protocol),
			Host:           t.Host,
			Port:           t.Port,
			Path:           t.Path,
			InsecureTLS:    t.InsecureTLS,
			TimeoutSeconds: t.TimeoutSeconds,
			TargetJSON:     string(tj),
		})
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, map[string]any{
		"Targets":     targetData,
		"CustomCAPEM": strings.TrimSpace(customCAPEM),
	})
	if err != nil {
		return "", fmt.Errorf("executing probe script template: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}
