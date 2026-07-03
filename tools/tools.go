//go:build tools

// This file keeps dev-only binaries in go.mod so "go mod tidy" preserves
// them. The version pinned in go.mod is the single source of truth — the
// Makefile runs the tool with a plain `go run` (no @version) so it always
// uses that pin. Bump with:
//
//	go get github.com/hashicorp/terraform-plugin-docs@vX.Y.Z
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
