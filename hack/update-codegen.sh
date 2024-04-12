#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

DIR="$(dirname "${BASH_SOURCE[0]}")"

ROOT_DIR="$(realpath "${DIR}/..")"

function deepcopy-gen() {
  go run k8s.io/code-generator/cmd/deepcopy-gen "$@"
}

function defaulter-gen() {
  go run k8s.io/code-generator/cmd/defaulter-gen "$@"
}

function client-gen() {
  go run k8s.io/code-generator/cmd/client-gen "$@"
}

function gen() {
  rm -rf \
    "${ROOT_DIR}/pkg/apis/v1alpha1"/zz_generated.*.go \
  echo "Generating deepcopy"
  deepcopy-gen \
    --input-dirs ./pkg/apis/v1alpha1/ \
    --trim-path-prefix github.com/wzshiming/jitdi/pkg/apis \
    --output-file-base zz_generated.deepcopy
  echo "Generating defaulter"
  defaulter-gen \
    --input-dirs ./pkg/apis/v1alpha1/ \
    --trim-path-prefix github.com/wzshiming/jitdi/pkg/apis \
    --output-file-base zz_generated.defaults

  rm -rf "${ROOT_DIR}/pkg/client"
  echo "Generating client"
  client-gen \
    --clientset-name versioned \
    --input-base "" \
    --input github.com/wzshiming/jitdi/pkg/apis/v1alpha1 \
    --output-package github.com/wzshiming/jitdi/pkg/client/clientset
}

cd "${ROOT_DIR}" && gen
