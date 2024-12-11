#!/bin/bash

NS_JUPYTER="$PLATFORM1"
NS_SATURN="$PLATFORM2"

# no-right-user
# Does not have access to any repo
NO_RIGHT_PWD="no-right-user-pwd"

POD_NAME=$(kubectl get pods -n $NS_JUPYTER -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
kubectl exec -i $POD_NAME -n $NS_JUPYTER -- gitea admin user create \
  --username $SANJI_USERNAME \
  --password "1$NO_RIGHT_PWD" \
  --email "$SANJI_USERNAME@syngit.io" 2>&1 > /dev/null
kubectl exec -i $POD_NAME -n $NS_JUPYTER -- gitea admin user change-password \
  --username $SANJI_USERNAME \
  --password $NO_RIGHT_PWD \
  --must-change-password=false 2>&1 > /dev/null

POD_NAME=$(kubectl get pods -n $NS_SATURN -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
kubectl exec -i $POD_NAME -n $NS_SATURN -- gitea admin user create \
  --username $SANJI_USERNAME \
  --password "1$NO_RIGHT_PWD" \
  --email "$SANJI_USERNAME@syngit.io" 2>&1 > /dev/null
kubectl exec -i $POD_NAME -n $NS_SATURN -- gitea admin user change-password \
  --username $SANJI_USERNAME \
  --password $NO_RIGHT_PWD \
  --must-change-password=false 2>&1 > /dev/null


# chopper-jb
# Have access only to the blue repo on the jupyter gitea
CHOPPER_PWD="chopper-jb-pwd"

POD_NAME=$(kubectl get pods -n $NS_JUPYTER -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
kubectl exec -i $POD_NAME -n $NS_JUPYTER -- gitea admin user create \
  --username $CHOPPER_USERNAME \
  --password "1$CHOPPER_PWD" \
  --email "$CHOPPER_USERNAME@syngit.io" 2>&1 > /dev/null
kubectl exec -i $POD_NAME -n $NS_JUPYTER -- gitea admin user change-password \
  --username $CHOPPER_USERNAME \
  --password $CHOPPER_PWD \
  --must-change-password=false 2>&1 > /dev/null


# luffy-jbg-sb
# Have access to the blue and green repo on the jupyter gitea
# Have access to the blue repo on the saturn gitea
LUFFY_PWD="luffy-jbg-sb-pwd"

POD_NAME=$(kubectl get pods -n $NS_JUPYTER -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
kubectl exec -i $POD_NAME -n $NS_JUPYTER -- gitea admin user create \
  --username $LUFFY_USERNAME \
  --password "1$LUFFY_PWD" \
  --email "$LUFFY_USERNAME@syngit.io" 2>&1 > /dev/null

kubectl exec -i $POD_NAME -n $NS_JUPYTER -- gitea admin user change-password \
  --username $LUFFY_USERNAME \
  --password $LUFFY_PWD \
  --must-change-password=false 2>&1 > /dev/null

POD_NAME=$(kubectl get pods -n $NS_SATURN -l app.kubernetes.io/name=gitea -o jsonpath="{.items[0].metadata.name}")
kubectl exec -i $POD_NAME -n $NS_SATURN -- gitea admin user create \
  --username $LUFFY_USERNAME \
  --password "1$LUFFY_PWD" \
  --email "$LUFFY_USERNAME@syngit.io" 2>&1 > /dev/null

kubectl exec -i $POD_NAME -n $NS_SATURN -- gitea admin user change-password \
  --username $LUFFY_USERNAME \
  --password $LUFFY_PWD \
  --must-change-password=false 2>&1 > /dev/null