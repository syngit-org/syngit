#!/bin/bash
set -a
. test/utils/.env
set +a

export PREFIXED_PATH=./test/utils/gitea

export NAMESPACE="$PLATFORM1"
$PREFIXED_PATH/delete-gitea-repos.sh
$PREFIXED_PATH/setup-gitea-repos.sh

export NAMESPACE="$PLATFORM2"
$PREFIXED_PATH/delete-gitea-repos.sh
$PREFIXED_PATH/setup-gitea-repos.sh

# Bind gitea user
export NAMESPACE="$PLATFORM1"
$PREFIXED_PATH/setup-gitea-bind-platform1.sh
export NAMESPACE="$PLATFORM2"
$PREFIXED_PATH/setup-gitea-bind-platform2.sh