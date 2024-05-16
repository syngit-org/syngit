#!/bin/bash

tmp_path=/tmp/k8s-webhook-server/serving-certs
# Generate the certificates
cd ${tmp_path}
./gen-certs-serv-cli.sh > /dev/null
cd -

# Encode certificates to base64
server_crt_base64=$(cat ${tmp_path}/server.crt | base64 | tr -d '\n')
server_key_base64=$(cat ${tmp_path}/server.key | base64 | tr -d '\n')
client_crt_base64=$(cat ${tmp_path}/client.crt | base64 | tr -d '\n')
client_key_base64=$(cat ${tmp_path}/client.key | base64 | tr -d '\n')

# Update the Secret
sed -i.bak -e "/server.crt:/c\  server.crt: $server_crt_base64" \
           -e "/server.key:/c\  server.key: $server_key_base64" \
           -e "/tls.crt:/c\  tls.crt: $client_crt_base64" \
           -e "/tls.key:/c\  tls.key: $client_key_base64" secret.yaml

# Remove existing caBundle lines if they exist
sed -i.bak '/^ *caBundle:.*/d' manifests.yaml

# Update the webhook configuration
sed -i.bak '/^ *clientConfig:/ {
  N
  s/\(^ *clientConfig:\)/\1\n    caBundle: '"$client_crt_base64"'/
}' manifests.yaml

# Clean up temporary files created by sed
rm -f secret.yaml.bak manifests.yaml.bak