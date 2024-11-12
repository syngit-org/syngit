#!/bin/bash

mv secret.yaml.bak secret.yaml
mv manifests.yaml.bak manifests.yaml

conversion_path="../crd/patches"
for file in "$conversion_path"/*; do
  # Get the base name of the file
  filename=$(basename "$file")
  
  # Check if the filename starts with "webhook_in"
  if [[ $filename != webhook_in* || $filename != *.yaml ]]; then
    continue
  fi
  
  mv "$conversion_path/$filename.bak" "$conversion_path/$filename"
done