#!/bin/bash
# Install cert-manager in the current cluster and wait for its webhook to be ready.
# The install is idempotent: it is skipped when the requested version is already deployed.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <CERT_MANAGER_VERSION>"
    exit 1
fi

set -e

CERT_MANAGER_VERSION=$1
RELEASE_URL="https://github.com/cert-manager/cert-manager/releases/download/$CERT_MANAGER_VERSION"
NAMESPACE="cert-manager"
DEPLOYMENTS="cert-manager cert-manager-webhook cert-manager-cainjector"

READINESS_PROBE="apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: webhook-readiness-probe
  namespace: default
spec:
  selfSigned: {}"

INSTALLED_VERSION=$(kubectl get deployment cert-manager -n $NAMESPACE \
    -o jsonpath='{.metadata.labels.app\.kubernetes\.io/version}' 2> /dev/null || true)

if [ "$INSTALLED_VERSION" = "$CERT_MANAGER_VERSION" ]; then
    echo "cert-manager $CERT_MANAGER_VERSION is already installed"
else
    echo "Installing cert-manager $CERT_MANAGER_VERSION"
    kubectl apply -f "$RELEASE_URL/cert-manager.crds.yaml"
    kubectl apply -f "$RELEASE_URL/cert-manager.yaml"
fi

for deployment in $DEPLOYMENTS; do
    kubectl wait --for=condition=Available "deployment/$deployment" -n $NAMESPACE --timeout=180s
done

# The deployments being available does not mean that the webhook already serves
# traffic (the CA bundle still has to be injected). Dry-run an Issuer until it is accepted.
for _ in $(seq 1 24); do
    if echo "$READINESS_PROBE" | kubectl apply --dry-run=server -f - > /dev/null 2>&1; then
        echo "cert-manager $CERT_MANAGER_VERSION is ready"
        exit 0
    fi
    echo "cert-manager webhook not ready yet, retrying in 5s..."
    sleep 5
done

echo "cert-manager webhook did not become ready within 120s" >&2
exit 1
