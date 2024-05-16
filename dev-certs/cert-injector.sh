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
sed -i.bak '/^ *caBundle:.*/d' webhook.yaml

# Update the webhook configuration
sed -i.bak '/^ *clientConfig:/ {
  N
  s/\(^ *clientConfig:\)/\1\n    caBundle: '"$client_crt_base64"'/
}' webhook.yaml

# Clean up temporary files created by sed
rm -f secret.yaml.bak webhook.yaml.bak

# Ask for the user to k apply the changes
read -p "Do you want to apply the changes to the Secret and webhook configuration? (y/n): " choice
if [ "$choice" = "y" ]; then
    kubectl apply -f secret.yaml --force
    kubectl delete -f webhook.yaml
    kubectl apply -f webhook.yaml --force
    echo "Changes applied successfully."
elif [ "$choice" = "n" ]; then
    echo "Changes not applied."
else
    echo "Invalid choice. Changes not applied."
fi