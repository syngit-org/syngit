#!/bin/bash

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
CN = webhook-service-client

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = syngit-webhook-service.syngit.svc
DNS.2 = syngit-remote-syncer-webhook-service.syngit.svc
IP.1 = 172.17.0.1
IP.2 = 127.0.0.1
EOF

# Generate CA key and certificate
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -days 365 \
  -subj "/C=AU/CN=webhook-service-CA" \
  -out ca.crt

# Generate server key and certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/C=AU/CN=webhook-service"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 365 -extfile openssl.cnf -extensions v3_req

# Generate client key and certificate
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr \
  -subj "/C=AU/CN=webhook-service-client"
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt -days 365 -extfile openssl.cnf -extensions v3_req

echo
echo ">> base64 server caBundle:"
cat server.crt | base64 | tr -d '\n'
echo
echo ">> base64 server key:"
cat server.key | base64 | tr -d '\n'

echo
echo ">> base64 client caBundle:"
cat client.crt | base64 | tr -d '\n'
echo
echo ">> base64 client key:"
cat client.key | base64 | tr -d '\n'

mv ca.crt tls.crt
mv ca.key tls.key
