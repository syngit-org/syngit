#!/bin/bash

crd_patches_path=$1/../crd/patches

# Backup files
if [ ! -f "$1/manifests.bak" ]; then
  cp $1/manifests.yaml $1/manifests.yaml.bak
  cp $1/secret.yaml $1/secret.yaml.bak

  for file in "$crd_patches_path"/*; do
    # Get the base name of the file
    filename=$(basename "$file")
    
    # Check if the filename starts with "webhook_in"
    if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
      continue
    fi
    
    cp "$crd_patches_path/$filename" "$crd_patches_path/$filename.bak"
  done
fi

# Generate the certificates
$1/gen-certs-serv-cli.sh
tmp_path=/tmp/k8s-webhook-server/serving-certs

# Encode certificates to base64
server_crt_base64=$(cat ${tmp_path}/server.crt | base64 | tr -d '\n')
server_key_base64=$(cat ${tmp_path}/server.key | base64 | tr -d '\n')
client_crt_base64=$(cat ${tmp_path}/client.crt | base64 | tr -d '\n')
client_key_base64=$(cat ${tmp_path}/client.key | base64 | tr -d '\n')

# Update the Secret
sed -i -e "/server.crt:/c\  server.crt: $server_crt_base64" \
           -e "/server.key:/c\  server.key: $server_key_base64" \
           -e "/tls.crt:/c\  tls.crt: $client_crt_base64" \
           -e "/tls.key:/c\  tls.key: $client_key_base64" $1/secret.yaml

# Remove existing caBundle lines if they exist
sed -i '/^ *caBundle:.*/d' $1/manifests.yaml

# Update the webhook configuration
sed -i '/^ *clientConfig:/ {
  N
  s/\(^ *clientConfig:\)/\1\n    caBundle: '"$client_crt_base64"'/
}' $1/manifests.yaml

# Update the conversion webhook configuration
for file in "$crd_patches_path"/*; do
  # Get the base name of the file
  filename=$(basename "$file")
  
  # Check if the filename starts with "webhook_in"
  if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
    continue
  fi
  
  sed -i '/^ *clientConfig:/ {
    N
    s/\(^ *clientConfig:\)/\1\n        caBundle: '"$client_crt_base64"'/
  }' "$crd_patches_path/$filename"
done
