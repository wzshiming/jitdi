//go:build tools
// +build tools

// Package tools is used to track binary dependencies with go modules
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package tools

import (
	// code-generator
	_ "k8s.io/code-generator"

	// controller-gen
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
