#!/bin/bash

if test -d /tmp/k8s-webhook-server/serving-certs/; then
  exit 0
fi

## Create the temp directory to welcome the certificates
mkdir -p /tmp/k8s-webhook-server/serving-certs/
cd /tmp/k8s-webhook-server/serving-certs/

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
DNS.1 = syngit-webhook-service.syngit.svc
DNS.2 = syngit-remote-syncer-webhook-service.syngit.svc
IP.1 = 172.17.0.1
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

echo
echo ">> base64 server caBundle:"
cat server.crt | base64 | tr -d '\n'
echo
echo ">> base64 server key:"
cat server.key | base64 | tr -d '\n'

mv server.crt tls.crt
mv server.key tls.key