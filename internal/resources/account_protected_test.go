package resources

import (
	"strings"
	"testing"
)

func TestProtectedAccountDeleteError_Root(t *testing.T) {
	summary, detail, ok := protectedAccountDeleteError("root")
	if !ok {
		t.Fatal(`expected a blocking error for "root", got ok=false`)
	}
	if !strings.Contains(summary, `"root"`) {
		t.Errorf("summary should quote the account name, got %q", summary)
	}
	if !strings.Contains(detail, "slurm_cluster") {
		t.Errorf("detail should point at slurm_cluster as the correct way to decommission a cluster, got: %s", detail)
	}
	if !strings.Contains(detail, "tofu state rm") {
		t.Errorf("detail should mention the state-rm escape hatch for un-managing without deleting, got: %s", detail)
	}
}

func TestProtectedAccountDeleteError_NonProtectedName(t *testing.T) {
	if _, _, ok := protectedAccountDeleteError("lab_physics"); ok {
		t.Error(`"lab_physics" must not be protected`)
	}
	if _, _, ok := protectedAccountDeleteError(""); ok {
		t.Error(`"" must not be protected`)
	}
}

func TestProtectedAccountDeleteError_CaseSensitive(t *testing.T) {
	// Slurm account names are case-sensitive -- "Root" is a distinct,
	// ordinary account name, not the built-in one. Don't false-positive.
	if _, _, ok := protectedAccountDeleteError("Root"); ok {
		t.Error(`"Root" (capital R) is not the built-in root account — must not be protected`)
	}
	if _, _, ok := protectedAccountDeleteError("ROOT"); ok {
		t.Error(`"ROOT" (uppercase) is not the built-in root account — must not be protected`)
	}
}

func TestProtectedAccountNames_NotEmpty(t *testing.T) {
	if len(protectedAccountNames) == 0 {
		t.Fatal("protectedAccountNames must not be empty — the guard would never fire")
	}
	for _, n := range protectedAccountNames {
		if n == "root" {
			return
		}
	}
	t.Error(`protectedAccountNames must include "root"`)
}
