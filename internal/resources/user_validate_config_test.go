package resources

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// assocOf is a small builder for an associationModel with only the fields
// validateUserConfigInvariants reads. The other fields stay as zero-value
// types.Int64{} / types.Set{} which are null — fine for these tests.
func assocOf(account types.String) associationModel {
	return associationModel{Account: account}
}

func TestValidateUserConfig_NoAssociationBlock(t *testing.T) {
	// Whole `association` set is null — equivalent to declaring no blocks.
	errs := validateUserConfigInvariants(true, false, nil, types.StringValue("acct1"))
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %d: %+v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Summary, "At least one association block") {
		t.Errorf("unexpected summary: %s", errs[0].Summary)
	}
	if got := errs[0].Path.String(); got != "association" {
		t.Errorf("want path=association, got %s", got)
	}
}

func TestValidateUserConfig_EmptyAssociationSet(t *testing.T) {
	// Set is known but empty — same error as null.
	errs := validateUserConfigInvariants(false, false, nil, types.StringValue("acct1"))
	if len(errs) != 1 || !strings.Contains(errs[0].Summary, "At least one association block") {
		t.Fatalf("want 'At least one association block' error, got %+v", errs)
	}
}

func TestValidateUserConfig_UnknownAssociations_Skips(t *testing.T) {
	// If the whole association set is unknown, defer to apply.
	errs := validateUserConfigInvariants(false, true, nil, types.StringValue("acct1"))
	if len(errs) != 0 {
		t.Fatalf("want no errors when associations unknown, got %+v", errs)
	}
}

func TestValidateUserConfig_DefaultAccountMatches(t *testing.T) {
	assocs := []associationModel{
		assocOf(types.StringValue("acct1")),
		assocOf(types.StringValue("acct2")),
	}
	errs := validateUserConfigInvariants(false, false, assocs, types.StringValue("acct2"))
	if len(errs) != 0 {
		t.Fatalf("want no errors when default_account matches, got %+v", errs)
	}
}

func TestValidateUserConfig_DefaultAccountMismatch(t *testing.T) {
	assocs := []associationModel{
		assocOf(types.StringValue("acct1")),
		assocOf(types.StringValue("acct2")),
	}
	errs := validateUserConfigInvariants(false, false, assocs, types.StringValue("missing"))
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %d: %+v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Summary, "default_account must match") {
		t.Errorf("unexpected summary: %s", errs[0].Summary)
	}
	if got := errs[0].Path.String(); got != "default_account" {
		t.Errorf("want path=default_account, got %s", got)
	}
	if !strings.Contains(errs[0].Detail, `"missing"`) {
		t.Errorf("detail should quote the offending account name, got: %s", errs[0].Detail)
	}
}

func TestValidateUserConfig_DefaultAccountNullOrUnknown_Skips(t *testing.T) {
	assocs := []associationModel{assocOf(types.StringValue("acct1"))}

	// default_account null — not user error, validator just defers.
	errs := validateUserConfigInvariants(false, false, assocs, types.StringNull())
	if len(errs) != 0 {
		t.Errorf("null default_account should not error here, got %+v", errs)
	}

	// default_account unknown (computed) — also defer.
	errs = validateUserConfigInvariants(false, false, assocs, types.StringUnknown())
	if len(errs) != 0 {
		t.Errorf("unknown default_account should defer, got %+v", errs)
	}
}

func TestValidateUserConfig_UnknownAccountInBlock_Skips(t *testing.T) {
	// One association has a computed account name. We can't say whether
	// default_account matches it or not, so defer to apply.
	assocs := []associationModel{
		assocOf(types.StringValue("acct1")),
		assocOf(types.StringUnknown()),
	}
	errs := validateUserConfigInvariants(false, false, assocs, types.StringValue("missing"))
	if len(errs) != 0 {
		t.Fatalf("unknown account should defer the cross-field check, got %+v", errs)
	}
}
