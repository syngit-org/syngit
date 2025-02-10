#!/bin/bash

mkdir -p $GITEA_TEMP_CERT_DIR

kubectl delete secret gitea-tls-secret -n $PLATFORM1 || true
kubectl delete secret gitea-tls-secret -n $PLATFORM2 || true

openssl genrsa -out $GITEA_TEMP_CERT_DIR/ca.key 2048
openssl req -x509 -new -nodes -key $GITEA_TEMP_CERT_DIR/ca.key -sha256 -days 365 \
  -out $GITEA_TEMP_CERT_DIR/ca.crt -subj "/CN=Gitea Root CA"

openssl genrsa -out $GITEA_TEMP_CERT_DIR/tls.key 2048
openssl req -new -key $GITEA_TEMP_CERT_DIR/tls.key -out $GITEA_TEMP_CERT_DIR/gitea.csr -subj "/CN=$1"
openssl x509 -req -in $GITEA_TEMP_CERT_DIR/gitea.csr \
  -CA $GITEA_TEMP_CERT_DIR/ca.crt -CAkey $GITEA_TEMP_CERT_DIR/ca.key -CAcreateserial \
  -out $GITEA_TEMP_CERT_DIR/tls.crt -days 365 -sha256 -extfile <(printf "subjectAltName=IP:$1")

kubectl create secret tls gitea-tls-secret --cert=$GITEA_TEMP_CERT_DIR/tls.crt --key=$GITEA_TEMP_CERT_DIR/tls.key -n $PLATFORM1
kubectl create secret tls gitea-tls-secret --cert=$GITEA_TEMP_CERT_DIR/tls.crt --key=$GITEA_TEMP_CERT_DIR/tls.key -n $PLATFORM2
