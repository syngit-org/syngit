#!/bin/bash

mv $1/secret.yaml.bak $1/secret.yaml
mv $1/manifests.yaml.bak $1/manifests.yaml
mv $1/dev-webhook.yaml.bak $1/dev-webhook.yaml

conversion_path=$1/../crd/patches
for file in "$conversion_path"/*; do
  # Get the base name of the file
  filename=$(basename "$file")
  
  # Check if the filename starts with "webhook_in"
  if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
    continue
  fi
  
  mv "$conversion_path/$filename.bak" "$conversion_path/$filename"
done