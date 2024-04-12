#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

DIR="$(dirname "${BASH_SOURCE[0]}")"

ROOT_DIR="$(realpath "${DIR}/..")"

function controller-gen() {
  go run sigs.k8s.io/controller-tools/cmd/controller-gen "$@"
}

function gen() {
  rm -rf \
    "${ROOT_DIR}/kustomize/crd/bases" \
    "${ROOT_DIR}/kustomize/rbac/rbac.yaml"
  echo "Generating crd/rbac"
  controller-gen \
    rbac:roleName=jitdi \
    crd:allowDangerousTypes=true \
    paths=./pkg/apis/v1alpha1/ \
    output:crd:artifacts:config=kustomize/crd/bases \
    output:rbac:artifacts:config=kustomize/rbac
}

cd "${ROOT_DIR}" && gen
