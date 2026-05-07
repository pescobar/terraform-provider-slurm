package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// emptyAccountModel returns an accountResourceModel with every TRES field null.
func emptyAccountModel(name string) accountResourceModel {
	nullTRES := types.SetNull(tresElemType())
	return accountResourceModel{
		ID:                types.StringValue(name),
		Name:              types.StringValue(name),
		Description:       types.StringNull(),
		Organization:      types.StringNull(),
		Parent:            types.StringNull(),
		Fairshare:         types.Int64Null(),
		DefaultQOS:        types.StringNull(),
		AllowedQOS:        types.ListNull(types.StringType),
		MaxJobs:           types.Int64Null(),
		MaxTRESPerJob:     nullTRES,
		MaxTRESPerNode:    nullTRES,
		MaxTRESMinsPerJob: nullTRES,
		GrpTRES:           nullTRES,
		GrpTRESMins:       nullTRES,
		GrpTRESRunMins:    nullTRES,
	}
}

// buildAssocMaxTRESFromAccountModel is a thin test wrapper that lets the
// existing TRES-from-account-model tests exercise the shared helper without
// having to spell out all six attributes at every call site.
func buildAssocMaxTRESFromAccountModel(ctx context.Context, m accountResourceModel) *client.AssociationMaxTRES {
	return buildAssocMaxTRES(ctx,
		m.MaxTRESPerJob, m.MaxTRESPerNode, m.MaxTRESMinsPerJob,
		m.GrpTRES, m.GrpTRESMins, m.GrpTRESRunMins,
	)
}

// ============================================================================
// buildAssocMaxTRES — exercised via the account model so we cover the same
// six fields slurm_account uses (slurm_user shares the same helper).
// ============================================================================

func TestBuildAssocMaxTRES_FromAccount_NilWhenAllNull(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	if result := buildAssocMaxTRESFromAccountModel(ctx, m); result != nil {
		t.Errorf("expected nil when all TRES fields are null, got %#v", result)
	}
}

func TestBuildAssocMaxTRES_FromAccount_MaxTRESPerJob(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.MaxTRESPerJob = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 32},
		{Type: "mem", Count: 65536},
	})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Per == nil || len(result.Per.Job) != 2 {
		t.Fatalf("expected 2 per.job entries, got %v", result.Per)
	}
}

func TestBuildAssocMaxTRES_FromAccount_MaxTRESPerNode(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.MaxTRESPerNode = buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 4},
	})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Per == nil || len(result.Per.Node) != 1 {
		t.Fatalf("expected 1 per.node entry, got %v", result.Per)
	}
	if result.Per.Node[0].Name != "gpu" {
		t.Errorf("expected name 'gpu', got %q", result.Per.Node[0].Name)
	}
}

func TestBuildAssocMaxTRES_FromAccount_MaxTRESMinsPerJob(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.MaxTRESMinsPerJob = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 480},
	})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Minutes == nil || result.Minutes.Per == nil || len(result.Minutes.Per.Job) != 1 {
		t.Fatalf("expected 1 minutes.per.job entry, got %v", result.Minutes)
	}
	if result.Minutes.Per.Job[0].Count != 480 {
		t.Errorf("expected count 480, got %d", result.Minutes.Per.Job[0].Count)
	}
}

func TestBuildAssocMaxTRES_FromAccount_GrpTRES(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.GrpTRES = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 256},
	})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Total) != 1 {
		t.Fatalf("expected 1 total entry, got %d", len(result.Total))
	}
	if result.Total[0].Count != 256 {
		t.Errorf("expected count 256, got %d", result.Total[0].Count)
	}
}

func TestBuildAssocMaxTRES_FromAccount_GrpTRESMins(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.GrpTRESMins = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 153600},
	})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Group == nil || len(result.Group.Minutes) != 1 {
		t.Fatalf("expected 1 group.minutes entry, got %v", result.Group)
	}
	if result.Group.Minutes[0].Count != 153600 {
		t.Errorf("expected count 153600, got %d", result.Group.Minutes[0].Count)
	}
}

func TestBuildAssocMaxTRES_FromAccount_GrpTRESRunMins(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.GrpTRESRunMins = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 76800},
	})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Group == nil || len(result.Group.Active) != 1 {
		t.Fatalf("expected 1 group.active entry, got %v", result.Group)
	}
	if result.Group.Active[0].Count != 76800 {
		t.Errorf("expected count 76800, got %d", result.Group.Active[0].Count)
	}
}

func TestBuildAssocMaxTRES_FromAccount_AllFields(t *testing.T) {
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.MaxTRESPerJob = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 8}})
	m.MaxTRESPerNode = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 4}})
	m.MaxTRESMinsPerJob = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 480}})
	m.GrpTRES = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 256}})
	m.GrpTRESMins = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 153600}})
	m.GrpTRESRunMins = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 76800}})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil {
		t.Fatal("expected non-nil result for model with all TRES fields set")
	}
	if result.Per == nil || len(result.Per.Job) != 1 {
		t.Error("expected per.job to be populated")
	}
	if result.Per == nil || len(result.Per.Node) != 1 {
		t.Error("expected per.node to be populated")
	}
	if result.Minutes == nil || result.Minutes.Per == nil || len(result.Minutes.Per.Job) != 1 {
		t.Error("expected minutes.per.job to be populated")
	}
	if len(result.Total) != 1 {
		t.Error("expected total (GrpTRES) to be populated")
	}
	if result.Group == nil || len(result.Group.Minutes) != 1 {
		t.Error("expected group.minutes (GrpTRESMins) to be populated")
	}
	if result.Group == nil || len(result.Group.Active) != 1 {
		t.Error("expected group.active (GrpTRESRunMins) to be populated")
	}
}

func TestBuildAssocMaxTRES_FromAccount_PerAndGrpShareGroupStruct(t *testing.T) {
	// GrpTRESMins and GrpTRESRunMins share the same Group sub-struct;
	// verify that setting both doesn't clobber either.
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.GrpTRESMins = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 1000}})
	m.GrpTRESRunMins = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 500}})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil || result.Group == nil {
		t.Fatal("expected non-nil group")
	}
	if len(result.Group.Minutes) != 1 || result.Group.Minutes[0].Count != 1000 {
		t.Errorf("GrpTRESMins: expected count 1000, got %v", result.Group.Minutes)
	}
	if len(result.Group.Active) != 1 || result.Group.Active[0].Count != 500 {
		t.Errorf("GrpTRESRunMins: expected count 500, got %v", result.Group.Active)
	}
}

func TestBuildAssocMaxTRES_FromAccount_MaxTRESPerJobAndPerNodeSharePerStruct(t *testing.T) {
	// MaxTRESPerJob and MaxTRESPerNode share the same Per sub-struct;
	// verify that setting both doesn't clobber either.
	ctx := context.Background()
	m := emptyAccountModel("acct")
	m.MaxTRESPerJob = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 64}})
	m.MaxTRESPerNode = buildTRESSet(t, []client.TRES{{Type: "cpu", Count: 16}})
	result := buildAssocMaxTRESFromAccountModel(ctx, m)
	if result == nil || result.Per == nil {
		t.Fatal("expected non-nil per")
	}
	if len(result.Per.Job) != 1 || result.Per.Job[0].Count != 64 {
		t.Errorf("MaxTRESPerJob: expected count 64, got %v", result.Per.Job)
	}
	if len(result.Per.Node) != 1 || result.Per.Node[0].Count != 16 {
		t.Errorf("MaxTRESPerNode: expected count 16, got %v", result.Per.Node)
	}
}
