#!/bin/bash

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <INPUT_FILE> <OUTPUT_FILE> <WEBHOOK_HOST>"
    exit 1
fi

INPUT_FILE=$1
OUTPUT_FILE=$2
WEBHOOK_HOST=$3

awk -v base_url="https://$WEBHOOK_HOST" '
# When we encounter clientConfig, we start tracking
/clientConfig/ {
    inside_block = 1;
    print "  clientConfig:";
    next;
}

# Capture the caBundle line
/caBundle:/ && inside_block {
    print $0;
    next;
}

# Skip over service, name, and namespace lines
/service:/ && inside_block {
    next;
}

/name:/ && inside_block {
    next;
}

# Skip namespace line
/namespace:/ && inside_block {
    next;
}

# Capture the path and replace it with url
/path:/ && inside_block {
    split($0, parts, ": ");
    print "    url: " base_url parts[2];
    inside_block = 0;  # End the block after processing path
    next;
}

# Print all other lines
{
    print $0;
}
' "$INPUT_FILE" > "$OUTPUT_FILE"
# echo "Conversion completed. Output written to $OUTPUT_FILE"

CRDS_PATH=config/crd/patches
CRD_WEBHOOK_FILES=$(ls $CRDS_PATH | grep webhook)
for crd_file in $CRD_WEBHOOK_FILES
do
cp "$CRDS_PATH/$crd_file" "$CRDS_PATH/$crd_file.bak"

awk -v base_url="https://$WEBHOOK_HOST" '
# When we encounter clientConfig, we start tracking
/clientConfig/ {
    inside_block = 1;
    print "      clientConfig:";
    next;
}

# Skip over service, name, and namespace lines
/service:/ && inside_block {
    next;
}

/name:/ && inside_block {
    next;
}

# Skip namespace line
/namespace:/ && inside_block {
    next;
}

# Capture the path and replace it with url
/path:/ && inside_block {
    split($0, parts, ": ");
    print "        url: " base_url parts[2];
    inside_block = 0;  # End the block after processing path
    next;
}

# Print all other lines
{
    print $0;
}
' "$CRDS_PATH/$crd_file" > "$CRDS_PATH/$crd_file.tmp"

mv "$CRDS_PATH/$crd_file.tmp" "$CRDS_PATH/$crd_file"  

# echo "Conversion completed. Output written to $CRDS_PATH/$crd_file"
done

