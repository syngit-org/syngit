#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <WEBHOOK_HOST>"
    exit 1
fi

WEBHOOK_HOST=$1
WEBHOOK_FILE="config/local/run/webhook/webhook.yaml"

awk -v base_url="https://$WEBHOOK_HOST:9443" '
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
' "$WEBHOOK_FILE" > "$WEBHOOK_FILE.tmp"

mv "$WEBHOOK_FILE.tmp" "$WEBHOOK_FILE"  

CRDS_PATH=config/local/run/crd/patches
CRD_WEBHOOK_FILES=$(ls $CRDS_PATH | grep webhook)
for crd_file in $CRD_WEBHOOK_FILES
do
awk -v base_url="https://$WEBHOOK_HOST:9443" '
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
    inside_block = 0;
    next;
}

# Print all other lines
{
    print $0;
}
' "$CRDS_PATH/$crd_file" > "$CRDS_PATH/$crd_file.tmp"

mv "$CRDS_PATH/$crd_file.tmp" "$CRDS_PATH/$crd_file"  

done

