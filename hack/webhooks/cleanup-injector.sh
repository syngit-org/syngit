#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <CERT_DIR>"
    exit 1
fi

CERT_DIR=$1

rm -rf config/local/run/webhook/webhook.yaml
rm -rf config/local/run/crd
rm -rf config/local/deploy/webhook/webhook.yaml
rm -rf config/local/deploy/crd
rm -rf $CERT_DIR