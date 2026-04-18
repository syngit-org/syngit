#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <CERT_DIR>"
    exit 1
fi

CERT_DIR=$1

sed -i '/caBundle:/d' config/local/run/webhook/webhook.yaml || true
sed -i '/caBundle:/d' config/local/run/crd/patches/webhook_in_remotesyncers.yaml || true
sed -i 's/\(tls\.crt: \).*/\1/; s/\(tls\.key: \).*/\1/' config/local/deploy/webhook/secret.yaml || true

rm -rf config/local/run/webhook/webhook.yaml
rm -rf config/local/run/crd
rm -rf config/local/deploy/webhook/webhook.yaml
rm -rf config/local/deploy/crd
rm -rf $CERT_DIR