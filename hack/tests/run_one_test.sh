#!/bin/bash

# Check if the argument is provided
if [ -z "$1" ]; then
  echo "Usage: $0 TEST_NUMBER"
  exit 1
fi

TEST_NUMBER="$1"
DIRECTORY="test/e2e/syngit"

# Iterate through the files in the directory
for FILE in "$DIRECTORY"/*; do
  # Extract the base name of the file
  BASENAME=$(basename "$FILE")
  
  # Skip the "e2e_suite_test.go" file
  if [[ "$BASENAME" == "e2e_suite_test.go" ]]; then
    continue
  fi

  # Check if the file name does not start with TEST_NUMBER
  if [[ ! $BASENAME == $TEST_NUMBER* ]]; then
    # Add ".ignore" to the file name if not already present
    if [[ $BASENAME != *.ignore ]]; then
      mv "$FILE" "$FILE.ignore"
    fi
  fi
done
