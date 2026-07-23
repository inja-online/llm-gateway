#!/usr/bin/env bash
# Generate localhost TLS material for Claude Code → gateway (HTTPS).
#
# Output (default: <repo>/examples/certs/):
#   localhost.pem      leaf cert (also use as NODE_EXTRA_CA_CERTS for self-signed)
#   localhost-key.pem  private key
#
# Prefer mkcert (trusted by system browsers/Node). Falls back to openssl SAN cert.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$ROOT/examples/certs}"
mkdir -p "$OUT"
CERT="$OUT/localhost.pem"
KEY="$OUT/localhost-key.pem"

if command -v mkcert >/dev/null 2>&1; then
  echo "using mkcert (installs local CA if needed)…"
  mkcert -install >/dev/null 2>&1 || true
  mkcert -cert-file "$CERT" -key-file "$KEY" localhost 127.0.0.1 ::1
  echo "wrote $CERT"
  echo "wrote $KEY"
  echo "mkcert CA is trusted by the system; Claude Code should accept https://127.0.0.1:8787"
  exit 0
fi

echo "mkcert not found — generating self-signed cert with openssl"
echo "  tip: brew install mkcert && mkcert -install  (better trust for Node/Claude Code)"
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl required" >&2
  exit 1
fi

CONF="$(mktemp)"
trap 'rm -f "$CONF"' EXIT
cat >"$CONF" <<'EOF'
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
x509_extensions = v3_req

[dn]
CN = localhost

[v3_req]
subjectAltName = @alt_names
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth

[alt_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout "$KEY" \
  -out "$CERT" \
  -days 825 \
  -config "$CONF" 2>/dev/null

chmod 600 "$KEY"
chmod 644 "$CERT"
echo "wrote $CERT"
echo "wrote $KEY"
echo
echo "Self-signed: point Claude Code at the cert so TLS verifies:"
echo "  export NODE_EXTRA_CA_CERTS=$CERT"
echo "  export SSL_CERT_FILE=$CERT   # some runtimes"
echo "  export ANTHROPIC_BASE_URL=https://127.0.0.1:8787"
