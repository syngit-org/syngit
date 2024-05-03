#!/bin/bash

# Determine the directory where the script is located
SCRIPT_DIR=$(dirname "$(realpath "$0")")

# Certificate and private key file paths
CERT_FILE="/tmp/k8s-webhook-server/serving-certs/tls.crt"
KEY_FILE="/tmp/k8s-webhook-server/serving-certs/tls.key"

# Check if the certificate and key files exist
if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
  echo "Certificate file $CERT_FILE or key file $KEY_FILE not found."
  exit 1
fi

# Read certificate and key content
CERT_DATA=$(cat "$CERT_FILE" | base64 -w 0)
KEY_DATA=$(cat "$KEY_FILE" | base64 -w 0)

# Generate Secret YAML
cat <<EOF >"$SCRIPT_DIR/secret.yaml"
apiVersion: v1
kind: Secret
metadata:
  name: webhook-server-cert
type: kubernetes.io/tls
data:
  tls.crt: $CERT_DATA
  tls.key: $KEY_DATA
EOF

echo "Secret YAML file created: secret.yaml"
