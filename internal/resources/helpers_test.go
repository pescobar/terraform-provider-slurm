package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// importTestSchema is the minimal schema needed by importStateByName: two
// string attributes (id, name). Mirrors what every real resource exposes.
func importTestSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true},
			"name": schema.StringAttribute{Required: true},
		},
	}
}

// ---------------------------------------------------------------------------
// slurmIntFromInt64
// ---------------------------------------------------------------------------

func TestSlurmIntFromInt64_Null(t *testing.T) {
	if got := slurmIntFromInt64(types.Int64Null()); got != nil {
		t.Errorf("expected nil for null input, got %#v", got)
	}
}

func TestSlurmIntFromInt64_Unknown(t *testing.T) {
	if got := slurmIntFromInt64(types.Int64Unknown()); got != nil {
		t.Errorf("expected nil for unknown input, got %#v", got)
	}
}

func TestSlurmIntFromInt64_Value(t *testing.T) {
	got := slurmIntFromInt64(types.Int64Value(42))
	if got == nil {
		t.Fatal("expected non-nil for valued input")
	}
	if got.Number != 42 {
		t.Errorf("Number: got %d, want 42", got.Number)
	}
	if !got.Set {
		t.Error("Set: got false, want true")
	}
	if got.Infinite {
		t.Error("Infinite: got true, want false")
	}
}

func TestSlurmIntFromInt64_Zero(t *testing.T) {
	// Zero is a legitimate value distinct from null — must not be coerced to nil.
	got := slurmIntFromInt64(types.Int64Value(0))
	if got == nil {
		t.Fatal("expected non-nil for zero value (zero is not null)")
	}
	if got.Number != 0 || !got.Set {
		t.Errorf("got %#v, want {Number:0, Set:true}", got)
	}
}

func TestSlurmIntFromInt64_Negative(t *testing.T) {
	got := slurmIntFromInt64(types.Int64Value(-5))
	if got == nil || got.Number != -5 || !got.Set {
		t.Errorf("got %#v, want {Number:-5, Set:true}", got)
	}
}

// ---------------------------------------------------------------------------
// intPtrFromInt64
// ---------------------------------------------------------------------------

func TestIntPtrFromInt64_Null(t *testing.T) {
	if got := intPtrFromInt64(types.Int64Null()); got != nil {
		t.Errorf("expected nil for null input, got %d", *got)
	}
}

func TestIntPtrFromInt64_Unknown(t *testing.T) {
	if got := intPtrFromInt64(types.Int64Unknown()); got != nil {
		t.Errorf("expected nil for unknown input, got %d", *got)
	}
}

func TestIntPtrFromInt64_Value(t *testing.T) {
	got := intPtrFromInt64(types.Int64Value(7))
	if got == nil {
		t.Fatal("expected non-nil for valued input")
	}
	if *got != 7 {
		t.Errorf("got %d, want 7", *got)
	}
}

func TestIntPtrFromInt64_Zero(t *testing.T) {
	// Zero is a legitimate value (e.g. fairshare=0 is valid in Slurm).
	got := intPtrFromInt64(types.Int64Value(0))
	if got == nil {
		t.Fatal("expected non-nil for zero value")
	}
	if *got != 0 {
		t.Errorf("got %d, want 0", *got)
	}
}

// ---------------------------------------------------------------------------
// configureClient
// ---------------------------------------------------------------------------

func TestConfigureClient_NilProviderData(t *testing.T) {
	// Framework calls Configure twice: once before provider Configure runs
	// (ProviderData == nil) and once after. The first call must be a no-op.
	req := resource.ConfigureRequest{ProviderData: nil}
	resp := &resource.ConfigureResponse{}
	got := configureClient(req, resp)
	if got != nil {
		t.Errorf("expected nil for nil ProviderData, got %#v", got)
	}
	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected diagnostic for nil ProviderData: %v", resp.Diagnostics)
	}
}

func TestConfigureClient_ValidClient(t *testing.T) {
	c := client.NewClient("http://test", "tok", "linux", "v0.0.42")
	req := resource.ConfigureRequest{ProviderData: c}
	resp := &resource.ConfigureResponse{}
	got := configureClient(req, resp)
	if got != c {
		t.Errorf("expected the same client pointer back, got %#v", got)
	}
	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected diagnostic: %v", resp.Diagnostics)
	}
}

func TestConfigureClient_WrongType(t *testing.T) {
	req := resource.ConfigureRequest{ProviderData: "not a client"}
	resp := &resource.ConfigureResponse{}
	got := configureClient(req, resp)
	if got != nil {
		t.Errorf("expected nil for wrong type, got %#v", got)
	}
	if !resp.Diagnostics.HasError() {
		t.Error("expected diagnostic error for wrong ProviderData type")
	}
}

// ---------------------------------------------------------------------------
// importStateByName
// ---------------------------------------------------------------------------

// importStateByName needs a real tfsdk.State to write to. Build one against the
// minimal schema shared by every resource ({id, name} string attributes).
func newImportState() tfsdk.State {
	objType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":   tftypes.String,
			"name": tftypes.String,
		},
	}
	rawState := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, nil),
		"name": tftypes.NewValue(tftypes.String, nil),
	})
	return tfsdk.State{
		Raw: rawState,
		Schema: importTestSchema(),
	}
}

func TestImportStateByName_WritesIDAndName(t *testing.T) {
	ctx := context.Background()
	req := resource.ImportStateRequest{ID: "team_alpha"}
	resp := &resource.ImportStateResponse{State: newImportState()}

	importStateByName(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var name, id types.String
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("name"), &name)...)
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("error reading state attrs: %v", resp.Diagnostics)
	}

	if name.ValueString() != "team_alpha" {
		t.Errorf("name: got %q, want %q", name.ValueString(), "team_alpha")
	}
	if id.ValueString() != "team_alpha" {
		t.Errorf("id: got %q, want %q", id.ValueString(), "team_alpha")
	}
}
