

# Install Gitea using Helm with a NodePort service
helm install $HELM_RELEASE_NAME gitea-charts/gitea --version 10.6.0 -n $NAMESPACE -f $VALUES_FILE --create-namespace > /dev/null

# Wait for the Gitea Pod to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=gitea -n $NAMESPACE --timeout=300s > /dev/null
