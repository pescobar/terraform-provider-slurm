package resources

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSystemQOSWarning_KnownSystemName(t *testing.T) {
	w, ok := systemQOSWarning(types.StringValue("normal"))
	if !ok {
		t.Fatalf(`expected warning for "normal", got ok=false`)
	}
	if !strings.Contains(w.Summary, "system QOS") {
		t.Errorf("summary should mention system QOS, got %q", w.Summary)
	}
	if !strings.Contains(w.Detail, `"normal"`) {
		t.Errorf("detail should quote the QOS name, got: %s", w.Detail)
	}
	if !strings.Contains(w.Detail, "Slurmdbd query returned with empty list") {
		t.Errorf("detail should reference the failure mode users will hit, got: %s", w.Detail)
	}
}

func TestSystemQOSWarning_NonSystemName(t *testing.T) {
	if _, ok := systemQOSWarning(types.StringValue("standard")); ok {
		t.Error(`"standard" must not trigger the warning`)
	}
	if _, ok := systemQOSWarning(types.StringValue("priority")); ok {
		t.Error(`"priority" must not trigger the warning`)
	}
}

func TestSystemQOSWarning_NullOrUnknown_NoWarning(t *testing.T) {
	if _, ok := systemQOSWarning(types.StringNull()); ok {
		t.Error("null name must not warn — defer to apply")
	}
	if _, ok := systemQOSWarning(types.StringUnknown()); ok {
		t.Error("unknown name must not warn — defer to apply")
	}
}

func TestSystemQOSWarning_CaseSensitive(t *testing.T) {
	// Slurm QOS names are case-sensitive — "Normal" is a separate user QOS,
	// not the system one. Don't false-positive.
	if _, ok := systemQOSWarning(types.StringValue("Normal")); ok {
		t.Error(`"Normal" (capital N) is not a system QOS — must not warn`)
	}
	if _, ok := systemQOSWarning(types.StringValue("NORMAL")); ok {
		t.Error(`"NORMAL" (uppercase) is not a system QOS — must not warn`)
	}
}

func TestSystemQOSNames_NotEmpty(t *testing.T) {
	if len(systemQOSNames) == 0 {
		t.Fatal("systemQOSNames must not be empty — the warning would never fire")
	}
	// Spot-check the well-known one.
	for _, n := range systemQOSNames {
		if n == "normal" {
			return
		}
	}
	t.Error(`systemQOSNames must include "normal" (Slurm's auto-created default QOS)`)
}
