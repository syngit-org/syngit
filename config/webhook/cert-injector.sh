#!/bin/bash

# Backup files
if [ ! -f "$1.bak" ]; then
  cp manifests.yaml manifests.yaml.bak
  cp secret.yaml secret.yaml.bak

  for file in "$2"/*; do
    # Get the base name of the file
    filename=$(basename "$file")
    
    # Check if the filename starts with "webhook_in"
    if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
      continue
    fi
    
    cp "$2/$filename" "$2/$filename.bak"
  done
fi

# Generate CA

mkdir -p /tmp/k8s-webhook-server/serving-certs
tmp_path=/tmp/k8s-webhook-server/serving-certs
# Generate the certificates
cd ${tmp_path}
./gen-certs-serv-cli.sh &> /dev/null
cd -

# Encode certificates to base64
server_crt_base64=$(cat ${tmp_path}/server.crt | base64 | tr -d '\n')
server_key_base64=$(cat ${tmp_path}/server.key | base64 | tr -d '\n')
client_crt_base64=$(cat ${tmp_path}/client.crt | base64 | tr -d '\n')
client_key_base64=$(cat ${tmp_path}/client.key | base64 | tr -d '\n')

# Update the Secret
sed -i -e "/server.crt:/c\  server.crt: $server_crt_base64" \
           -e "/server.key:/c\  server.key: $server_key_base64" \
           -e "/tls.crt:/c\  tls.crt: $client_crt_base64" \
           -e "/tls.key:/c\  tls.key: $client_key_base64" secret.yaml

# Remove existing caBundle lines if they exist
sed -i '/^ *caBundle:.*/d' $1

# Update the webhook configuration
sed -i '/^ *clientConfig:/ {
  N
  s/\(^ *clientConfig:\)/\1\n    caBundle: '"$client_crt_base64"'/
}' $1

# Update the conversion webhook configuration
for file in "$2"/*; do
  # Get the base name of the file
  filename=$(basename "$file")
  
  # Check if the filename starts with "webhook_in"
  if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
    continue
  fi
  
  sed -i '/^ *clientConfig:/ {
    N
    s/\(^ *clientConfig:\)/\1\n        caBundle: '"$client_crt_base64"'/
  }' "$2/$filename"
done
