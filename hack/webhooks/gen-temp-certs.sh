#!/bin/bash

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <WEBHOOK_HOST> <CERT_DIR> (/tmp/k8s-webhook-server/serving-certs/)"
    exit 1
fi

WEBHOOK_HOST=$1
CERT_DIR=$2
ALT_NAME_TYPE="DNS.1"

## Create the temp directory to welcome the certificates
mkdir -p $CERT_DIR
cd $CERT_DIR

## Check if it's an IP address
if [[ $WEBHOOK_HOST =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  ALT_NAME_TYPE="IP.1"
fi

cat > openssl.cnf <<EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
C = AU

[v3_req]
subjectAltName = @alt_names

[alt_names]
$ALT_NAME_TYPE = $WEBHOOK_HOST
EOF

# Generate CA key and certificate
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -days 365 \
  -subj "/C=AU" \
  -out ca.crt \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,keyCertSign,cRLSign"

# Generate server key and certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -config openssl.cnf
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 \
  -extfile openssl.cnf -extensions v3_req

mv server.crt tls.crt
mv server.key tls.key