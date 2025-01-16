#!/bin/bash

DIRECTORY="test/e2e/syngit"

# Iterate through the files in the directory
for FILE in "$DIRECTORY"/*.ignore; do
  # Check if there are any matching files
  if [ -e "$FILE" ]; then
    # Remove the ".ignore" suffix
    NEW_NAME="${FILE%.ignore}"
    mv "$FILE" "$NEW_NAME"
  fi
done
