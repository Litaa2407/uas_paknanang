#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CERT_DIR="$ROOT_DIR/certs"
command -v openssl >/dev/null 2>&1 || { echo "OpenSSL belum terpasang." >&2; exit 1; }
mkdir -p "$CERT_DIR"
rm -f "$CERT_DIR"/ca.key "$CERT_DIR"/ca.crt "$CERT_DIR"/ca.srl \
  "$CERT_DIR"/campaign.* "$CERT_DIR"/notification.* "$CERT_DIR"/gateway.*

openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
  -keyout "$CERT_DIR/ca.key" -out "$CERT_DIR/ca.crt" \
  -subj "/CN=Crowdfunding Local Development CA"

issue_cert() {
  local name="$1" common_name="$2" extension="$3"
  openssl req -new -newkey rsa:2048 -nodes \
    -keyout "$CERT_DIR/$name.key" -out "$CERT_DIR/$name.csr" \
    -subj "/CN=$common_name"
  openssl x509 -req -in "$CERT_DIR/$name.csr" \
    -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
    -out "$CERT_DIR/$name.crt" -days 825 -sha256 -extfile "$extension"
  rm -f "$CERT_DIR/$name.csr"
}

cat > "$CERT_DIR/campaign.ext" <<'EXT'
subjectAltName=DNS:campaign-service,DNS:campaign-service-2,DNS:localhost,IP:127.0.0.1
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EXT
cat > "$CERT_DIR/notification.ext" <<'EXT'
subjectAltName=DNS:notification-service,DNS:localhost,IP:127.0.0.1
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EXT
cat > "$CERT_DIR/gateway.ext" <<'EXT'
subjectAltName=DNS:gateway
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
EXT

issue_cert campaign campaign-service "$CERT_DIR/campaign.ext"
issue_cert notification notification-service "$CERT_DIR/notification.ext"
issue_cert gateway gateway "$CERT_DIR/gateway.ext"

rm -f "$CERT_DIR"/*.ext "$CERT_DIR"/*.srl
chmod 644 "$CERT_DIR"/*.crt "$CERT_DIR/campaign.key" "$CERT_DIR/notification.key" "$CERT_DIR/gateway.key"
chmod 600 "$CERT_DIR/ca.key"

echo "Sertifikat development dibuat di: $CERT_DIR"
echo "Jangan commit folder certs/ atau gunakan sertifikat ini untuk production."
