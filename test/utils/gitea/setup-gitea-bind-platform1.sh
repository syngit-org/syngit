#!/bin/bash

SERVICE_PORT=$(kubectl get svc $SERVICE_NAME -n $NAMESPACE -o jsonpath="{.spec.ports[0].nodePort}")
NODE_IP=$(kubectl get nodes -o jsonpath="{.items[0].status.addresses[?(@.type=='InternalIP')].address}")

# Formulate the Gitea URL for API access
GITEA_URL="http://$NODE_IP:$SERVICE_PORT"

POD_NAME=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
TOKEN_RESPONSE=$(kubectl exec -i $POD_NAME -n $NAMESPACE -- gitea admin user generate-access-token \
  --username $ADMIN_USERNAME \
  --scopes "all" \
  --token-name bindtoken 2>/dev/null)

if [ "$TOKEN_RESPONSE" == "null" ]; then
  echo "Failed to generate token for $ADMIN_USERNAME user."
  exit 1
fi

ADMIN_TOKEN=$(echo "$TOKEN_RESPONSE" | sed -E 's/.*Access token was successfully created: ([a-f0-9]{40}).*/\1/')

. $PREFIXED_PATH/add-collaborator.sh

#
## CHOPPER
#
ADDED=$(add-collaborator $GITEA_URL $ADMIN_TOKEN "blue" $CHOPPER_USERNAME)
if [ "$ADDED" = "1" ]; then
  echo "User '$CHOPPER_USERNAME' failed to be added to repository 'blue' with 'write' access on $PLATFORM1."
  exit 1
fi



#
## LUFFY
#
add-collaborator $GITEA_URL $ADMIN_TOKEN "blue" $LUFFY_USERNAME
ADDED=$(add-collaborator $GITEA_URL $ADMIN_TOKEN "blue" $LUFFY_USERNAME)
if [ "$ADDED" = "1" ]; then
  echo "User '$LUFFY_USERNAME' failed to be added to repository 'blue' with 'write' access on $PLATFORM1."
  exit 1
fi

ADDED=$(add-collaborator $GITEA_URL $ADMIN_TOKEN "green" $LUFFY_USERNAME)
if [ "$ADDED" = "1" ]; then
  echo "User '$LUFFY_USERNAME' failed to be added to repository 'green' with 'write' access on $PLATFORM1."
  exit 1
fi
