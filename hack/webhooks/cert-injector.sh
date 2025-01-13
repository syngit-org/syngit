#!/bin/bash

if [ "$#" -ne 5 ]; then
    echo "Usage: $0 <WEBHOOK_PATH> <CRD_PATH> <LOCAL_PATH> <CERT_DIR> <WEBHOOK_HOST>"
    exit 1
fi

WEBHOOK_PATH=$1
CRD_PATH=$2
CRD_PATCHES_PATH=$CRD_PATH/patches
LOCAL_PATH=$3
CERT_DIR=$4
WEBHOOK_HOST=$5

LOCAL_WEBHOOK_PATH=$LOCAL_PATH/webhook/webhook.yaml
LOCAL_CRD_PATCHES_PATCH=$LOCAL_PATH/crd/patches

# Copy to local
cp $1/manifests.yaml $LOCAL_WEBHOOK_PATH
cp -r $2 $LOCAL_PATH/crd

# Generate the certificates
./hack/webhooks/gen-temp-certs.sh $WEBHOOK_HOST $CERT_DIR

# Encode certificates to base64
server_crt_base64=$(cat ${CERT_DIR}/tls.crt | base64 -w0)
server_key_base64=$(cat ${CERT_DIR}/tls.key | base64 -w0)
cabundle_crt_base64=$(cat ${CERT_DIR}/ca.crt | base64 -w0)
cabundle_key_base64=$(cat ${CERT_DIR}/ca.key | base64 -w0)

# Remove existing caBundle lines if they exist
sed -i '/^ *caBundle:.*/d' $LOCAL_WEBHOOK_PATH

# Update the webhook configuration
sed -i '/^ *clientConfig:/ {
  N
  s/\(^ *clientConfig:\)/\1\n    caBundle: '"$cabundle_crt_base64"'/
}' $LOCAL_WEBHOOK_PATH

# Update the conversion webhook configuration
for file in "$LOCAL_CRD_PATCHES_PATCH"/*; do
  # Get the base name of the file
  filename=$(basename "$file")
  
  # Check if the filename starts with "webhook_in"
  if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
    continue
  fi
  
sed -i '/^ *clientConfig:/ {
  N
  s/\(^ *clientConfig:\)/\1\n        caBundle: '"$cabundle_crt_base64"'/
}' "$LOCAL_CRD_PATCHES_PATCH/$filename"
done
