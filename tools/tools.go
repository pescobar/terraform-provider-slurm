//go:build tools

// This file pins dev-only binaries so "go mod tidy" keeps them in go.sum.
// Install with: go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.19.4
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
