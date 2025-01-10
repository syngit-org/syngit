#!/bin/bash

add-collaborator () {

set -a
. test/utils/.env
set +a

GITEA_URL=$1
ADMIN_TOKEN=$2
REPO=$3
COLLABORATOR=$4
ADD_COLLABORATOR_ENDPOINT="$GITEA_URL/api/v1/repos/$ADMIN_USERNAME/$REPO/collaborators/$COLLABORATOR"

# JSON payload for setting the access level
JSON_PAYLOAD=$(cat <<EOF
{
  "permission": "all"
}
EOF
)

# API request to add the user as a collaborator
response=$(curl -s -o /dev/null -w "%{http_code}" -X PUT -k \
  -H "Content-Type: application/json" \
  -H "Authorization: token $ADMIN_TOKEN" \
  -d "$JSON_PAYLOAD" \
  "$ADD_COLLABORATOR_ENDPOINT")

# Check the response code
if [ "$response" -eq 204 ]; then
  return 0
else
  return 1
fi

}