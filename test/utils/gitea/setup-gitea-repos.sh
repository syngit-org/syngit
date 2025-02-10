#!/bin/bash

# Get the Gitea service NodePort
SERVICE_PORT=$(kubectl get svc $SERVICE_NAME -n $NAMESPACE -o jsonpath="{.spec.ports[0].nodePort}")
NODE_IP=$(kubectl get nodes -o jsonpath="{.items[0].status.addresses[?(@.type=='InternalIP')].address}")

# Formulate the Gitea URL for API access
GITEA_URL="https://$NODE_IP:$SERVICE_PORT"

# Create an admin user using Gitea CLI inside the Gitea pod
POD_NAME=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
kubectl exec -i $POD_NAME -n $NAMESPACE -- gitea admin user create \
  --username $ADMIN_USERNAME \
  --password "1$ADMIN_PASSWORD" \
  --email $ADMIN_EMAIL \
  --admin 2>&1 > /dev/null

kubectl exec -i $POD_NAME -n $NAMESPACE -- gitea admin user change-password \
  --username $ADMIN_USERNAME \
  --password $ADMIN_PASSWORD \
  --must-change-password=false 2>&1 > /dev/null

TOKEN_RESPONSE=$(kubectl exec -i $POD_NAME -n $NAMESPACE -- gitea admin user generate-access-token \
  --username $ADMIN_USERNAME \
  --scopes "all" \
  --token-name mytoken-$RANDOM 2>/dev/null)

if [ "$TOKEN_RESPONSE" == "null" ]; then
  echo "Failed to generate token for syngituser user."
  exit 1
fi

GIT_TOKEN=$(echo "$TOKEN_RESPONSE" | sed -E 's/.*Access token was successfully created: ([a-f0-9]{40}).*/\1/')

#
# Create the blue repo
#
CREATE_REPO_ENDPOINT="$GITEA_URL/api/v1/user/repos"

JSON_PAYLOAD=$(cat <<EOF
{
  "name": "blue",
  "private": false,
  "auto_init": true,
  "description": "A new sample repository"
}
EOF
)

# Make the API call to create the repository
response=$(curl -s -o /dev/null -w "%{http_code}" -X POST -k \
  -H "Content-Type: application/json" \
  -d "$JSON_PAYLOAD" \
  "$CREATE_REPO_ENDPOINT?access_token=$GIT_TOKEN")

# Check the response code
if [ "$response" != 201 ]; then
  echo "Failed to create repository. HTTP status code: $response"
  exit 1
fi


#
# Create the green repo
#
CREATE_REPO_ENDPOINT="$GITEA_URL/api/v1/user/repos"

JSON_PAYLOAD=$(cat <<EOF
{
  "name": "green",
  "private": false,
  "auto_init": true,
  "description": "A new sample repository"
}
EOF
)

# Make the API call to create the repository
response=$(curl -s -o /dev/null -w "%{http_code}" -X POST -k \
  -H "Content-Type: application/json" \
  -d "$JSON_PAYLOAD" \
  "$CREATE_REPO_ENDPOINT?access_token=$GIT_TOKEN")

# Check the response code
if [ "$response" != 201 ]; then
  echo "Failed to create repository. HTTP status code: $response"
  exit 1
fi
