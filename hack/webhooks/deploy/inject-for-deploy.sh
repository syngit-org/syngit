#!/bin/bash

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <CERT_DIR> <WEBHOOK_PATH>"
    exit 1
fi

LOCAL_PATH=config/local/deploy
CERT_DIR=$1
WEBHOOK_PATH=$2

cp $WEBHOOK_PATH/service.yaml $LOCAL_PATH/service.yaml

server_crt_base64=$(cat ${CERT_DIR}/tls.crt | base64 | tr -d '\n')
server_key_base64=$(cat ${CERT_DIR}/tls.key | base64 | tr -d '\n')

# Update the Secret
sed -i -e "/tls.crt:/c\  tls.crt: $server_crt_base64" \
       -e "/tls.key:/c\  tls.key: $server_key_base64" $LOCAL_PATH/webhook/secret.yaml

