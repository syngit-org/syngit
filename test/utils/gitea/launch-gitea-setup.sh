#!/bin/bash
set -a
. test/utils/.env
set +a

echo "☕ Initializing gitea..."
export PREFIXED_PATH=./test/utils/gitea

# Add Gitea Helm repository and update
helm repo add gitea-charts https://dl.gitea.io/charts/ > /dev/null
helm repo update > /dev/null

kubectl create ns $PLATFORM1
kubectl create ns $PLATFORM2

# Generate certs
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
$PREFIXED_PATH/gitea-gen-cert.sh $NODE_IP

export NAMESPACE="$PLATFORM1"
export VALUES_FILE="$PREFIXED_PATH/helm-values-$PLATFORM1.yaml"
$PREFIXED_PATH/setup-gitea-install.sh

export NAMESPACE="$PLATFORM2"
export VALUES_FILE="$PREFIXED_PATH/helm-values-$PLATFORM2.yaml"
$PREFIXED_PATH/setup-gitea-install.sh

# Setup users
$PREFIXED_PATH/setup-gitea-users.sh

# Setup repos & repos bindings
$PREFIXED_PATH/reset-gitea-repos.sh