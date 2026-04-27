package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// emptyModel returns a qosResourceModel with every optional field set to its
// null/zero equivalent so callers only have to set the fields under test.
func emptyModel(name string) qosResourceModel {
	return qosResourceModel{
		ID:                      types.StringValue(name),
		Name:                    types.StringValue(name),
		Description:             types.StringNull(),
		Priority:                types.Int64Null(),
		MaxWallPJ:               types.Int64Null(),
		GrpWall:                 types.Int64Null(),
		GraceTime:               types.Int64Null(),
		UsageFactor:             types.Int64Null(),
		UsageThreshold:          types.Int64Null(),
		PreemptExemptTime:       types.Int64Null(),
		GrpJobs:                 types.Int64Null(),
		GrpSubmitJobs:           types.Int64Null(),
		MaxJobsPerUser:          types.Int64Null(),
		MaxSubmitJobsPerUser:    types.Int64Null(),
		MaxJobsPerAccount:       types.Int64Null(),
		MaxSubmitJobsPerAccount: types.Int64Null(),
		Flags:                   types.SetNull(types.StringType),
		PreemptList:             types.SetNull(types.StringType),
		PreemptMode:             types.SetNull(types.StringType),
		GrpTRES:                 types.SetNull(tresElemType()),
		GrpTRESMins:             types.SetNull(tresElemType()),
		MaxTRESPerJob:           types.SetNull(tresElemType()),
		MaxTRESMinsPerJob:       types.SetNull(tresElemType()),
		MaxTRESPerNode:          types.SetNull(tresElemType()),
		MaxTRESPerUser:          types.SetNull(tresElemType()),
		MaxTRESMinsPerUser:      types.SetNull(tresElemType()),
		MaxTRESPerAccount:       types.SetNull(tresElemType()),
		MaxTRESMinsPerAccount:   types.SetNull(tresElemType()),
		MinTRESPerJob:           types.SetNull(tresElemType()),
	}
}

// buildTRESSet constructs a types.Set of TRES objects for use in model fields.
// Pass name="" for standard types like cpu/mem that have no sub-name.
func buildTRESSet(t *testing.T, entries []client.TRES) types.Set {
	t.Helper()
	elems := make([]attr.Value, len(entries))
	for i, e := range entries {
		nameVal := types.StringNull()
		if e.Name != "" {
			nameVal = types.StringValue(e.Name)
		}
		obj, d := types.ObjectValue(tresAttrTypes(), map[string]attr.Value{
			"type":  types.StringValue(e.Type),
			"name":  nameVal,
			"count": types.Int64Value(e.Count),
		})
		if d.HasError() {
			t.Fatalf("buildTRESSet: %v", d)
		}
		elems[i] = obj
	}
	s, d := types.SetValue(tresElemType(), elems)
	if d.HasError() {
		t.Fatalf("buildTRESSet: %v", d)
	}
	return s
}

// buildStringSet constructs a types.Set of strings.
func buildStringSet(t *testing.T, values []string) types.Set {
	t.Helper()
	s, d := types.SetValueFrom(context.Background(), types.StringType, values)
	if d.HasError() {
		t.Fatalf("buildStringSet: %v", d)
	}
	return s
}

// ============================================================================
// apiTresListToSet
// ============================================================================

func TestApiTresListToSet_EmptyList(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	result := apiTresListToSet(ctx, nil, &diags)
	if !result.IsNull() {
		t.Error("expected null set for empty list, got non-null")
	}
	if diags.HasError() {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
}

func TestApiTresListToSet_StandardTRES(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	list := []client.TRES{
		{Type: "cpu", Count: 64},
		{Type: "mem", Count: 128000},
	}
	result := apiTresListToSet(ctx, list, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result.IsNull() {
		t.Fatal("expected non-null set for non-empty list")
	}
	if len(result.Elements()) != 2 {
		t.Errorf("expected 2 elements, got %d", len(result.Elements()))
	}

	// Extract and verify elements
	var models []tresModel
	result.ElementsAs(ctx, &models, false)
	found := map[string]int64{}
	for _, m := range models {
		found[m.Type.ValueString()] = m.Count.ValueInt64()
		if !m.Name.IsNull() {
			t.Errorf("expected null name for type %q, got %q", m.Type.ValueString(), m.Name.ValueString())
		}
	}
	if found["cpu"] != 64 {
		t.Errorf("expected cpu count 64, got %d", found["cpu"])
	}
	if found["mem"] != 128000 {
		t.Errorf("expected mem count 128000, got %d", found["mem"])
	}
}

func TestApiTresListToSet_GenericTRES(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	list := []client.TRES{
		{Type: "gres", Name: "gpu", Count: 8},
	}
	result := apiTresListToSet(ctx, list, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var models []tresModel
	result.ElementsAs(ctx, &models, false)
	if len(models) != 1 {
		t.Fatalf("expected 1 element, got %d", len(models))
	}
	if models[0].Type.ValueString() != "gres" {
		t.Errorf("expected type 'gres', got %q", models[0].Type.ValueString())
	}
	if models[0].Name.IsNull() || models[0].Name.ValueString() != "gpu" {
		t.Errorf("expected name 'gpu', got %v", models[0].Name)
	}
	if models[0].Count.ValueInt64() != 8 {
		t.Errorf("expected count 8, got %d", models[0].Count.ValueInt64())
	}
}

func TestApiTresListToSet_MixedTRES(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	list := []client.TRES{
		{Type: "cpu", Count: 32},
		{Type: "gres", Name: "gpu", Count: 4},
	}
	result := apiTresListToSet(ctx, list, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(result.Elements()) != 2 {
		t.Errorf("expected 2 elements, got %d", len(result.Elements()))
	}
}

// ============================================================================
// planTresListToAPI
// ============================================================================

func TestPlanTresListToAPI_NullSet(t *testing.T) {
	ctx := context.Background()
	s := types.SetNull(tresElemType())
	result := planTresListToAPI(ctx, s)
	if result != nil {
		t.Errorf("expected nil for null set, got %v", result)
	}
}

func TestPlanTresListToAPI_EmptySet(t *testing.T) {
	ctx := context.Background()
	s, d := types.SetValue(tresElemType(), []attr.Value{})
	if d.HasError() {
		t.Fatalf("setup: %v", d)
	}
	result := planTresListToAPI(ctx, s)
	if result != nil {
		t.Errorf("expected nil for empty set, got %v", result)
	}
}

func TestPlanTresListToAPI_StandardTRES(t *testing.T) {
	ctx := context.Background()
	s := buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 128},
	})
	result := planTresListToAPI(ctx, s)
	if len(result) != 1 {
		t.Fatalf("expected 1 element, got %d", len(result))
	}
	if result[0].Type != "cpu" {
		t.Errorf("expected type 'cpu', got %q", result[0].Type)
	}
	if result[0].Name != "" {
		t.Errorf("expected empty name, got %q", result[0].Name)
	}
	if result[0].Count != 128 {
		t.Errorf("expected count 128, got %d", result[0].Count)
	}
}

func TestPlanTresListToAPI_GenericTRES(t *testing.T) {
	ctx := context.Background()
	s := buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 4},
	})
	result := planTresListToAPI(ctx, s)
	if len(result) != 1 {
		t.Fatalf("expected 1 element, got %d", len(result))
	}
	if result[0].Name != "gpu" {
		t.Errorf("expected name 'gpu', got %q", result[0].Name)
	}
}

func TestPlanTresListToAPI_MixedTRES(t *testing.T) {
	ctx := context.Background()
	s := buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 64},
		{Type: "mem", Count: 256000},
		{Type: "gres", Name: "gpu", Count: 8},
	})
	result := planTresListToAPI(ctx, s)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
}

// ============================================================================
// modelToAPI — one test function per feature area
// ============================================================================

func TestModelToAPI_Minimal(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()
	m := emptyModel("testqos")
	qos := r.modelToAPI(ctx, m)

	if qos.Name != "testqos" {
		t.Errorf("expected name 'testqos', got %q", qos.Name)
	}
	if qos.Priority != nil {
		t.Error("expected nil priority for unset field")
	}
	if qos.Limits != nil {
		t.Error("expected nil limits when no limit fields are set")
	}
	if qos.Preempt != nil {
		t.Error("expected nil preempt when no preempt fields are set")
	}
	if len(qos.Flags) != 0 {
		t.Errorf("expected empty flags, got %v", qos.Flags)
	}
}

func TestModelToAPI_BasicFields(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.Description = types.StringValue("Test QOS")
	m.Priority = types.Int64Value(300)
	m.UsageFactor = types.Int64Value(2)
	m.UsageThreshold = types.Int64Value(50)
	qos := r.modelToAPI(ctx, m)

	if qos.Description != "Test QOS" {
		t.Errorf("expected description 'Test QOS', got %q", qos.Description)
	}
	if qos.Priority == nil || !qos.Priority.Set || qos.Priority.Number != 300 {
		t.Errorf("expected priority {300, set}, got %v", qos.Priority)
	}
	if qos.UsageFactor == nil || qos.UsageFactor.Number != 2 {
		t.Errorf("expected usage_factor 2, got %v", qos.UsageFactor)
	}
	if qos.UsageThreshold == nil || qos.UsageThreshold.Number != 50 {
		t.Errorf("expected usage_threshold 50, got %v", qos.UsageThreshold)
	}
}

func TestModelToAPI_Flags(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()
	m := emptyModel("testqos")
	m.Flags = buildStringSet(t, []string{"NO_DECAY", "DENY_LIMIT"})
	qos := r.modelToAPI(ctx, m)

	if len(qos.Flags) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(qos.Flags))
	}
}

func TestModelToAPI_WallClock(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.MaxWallPJ = types.Int64Value(1440)
	m.GrpWall = types.Int64Value(2880)
	qos := r.modelToAPI(ctx, m)

	if qos.Limits == nil || qos.Limits.Max == nil || qos.Limits.Max.WallClock == nil || qos.Limits.Max.WallClock.Per == nil {
		t.Fatal("expected wall clock limits to be set")
	}
	wc := qos.Limits.Max.WallClock.Per
	if wc.Job == nil || wc.Job.Number != 1440 {
		t.Errorf("expected max_wall_pj 1440, got %v", wc.Job)
	}
	if wc.QOS == nil || wc.QOS.Number != 2880 {
		t.Errorf("expected grp_wall 2880, got %v", wc.QOS)
	}
}

func TestModelToAPI_Preempt(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.PreemptList = buildStringSet(t, []string{"standard", "basic"})
	m.PreemptMode = buildStringSet(t, []string{"CANCEL"})
	m.PreemptExemptTime = types.Int64Value(300)
	qos := r.modelToAPI(ctx, m)

	if qos.Preempt == nil {
		t.Fatal("expected preempt to be set")
	}
	if len(qos.Preempt.List) != 2 {
		t.Errorf("expected 2 preempt list entries, got %d", len(qos.Preempt.List))
	}
	if len(qos.Preempt.Mode) != 1 || qos.Preempt.Mode[0] != "CANCEL" {
		t.Errorf("expected preempt mode [CANCEL], got %v", qos.Preempt.Mode)
	}
	if qos.Preempt.ExemptTime == nil || qos.Preempt.ExemptTime.Number != 300 {
		t.Errorf("expected preempt_exempt_time 300, got %v", qos.Preempt.ExemptTime)
	}
}

func TestModelToAPI_MaxTRESPerJob(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.MaxTRESPerJob = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 128},
		{Type: "gres", Name: "gpu", Count: 8},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits == nil || qos.Limits.Max == nil || qos.Limits.Max.TRES == nil || qos.Limits.Max.TRES.Per == nil {
		t.Fatal("expected TRES per-job limits to be set")
	}
	if len(qos.Limits.Max.TRES.Per.Job) != 2 {
		t.Errorf("expected 2 per-job TRES entries, got %d", len(qos.Limits.Max.TRES.Per.Job))
	}
}

func TestModelToAPI_MaxTRESPerNode(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.MaxTRESPerNode = buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 4},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits.Max.TRES.Per.Node == nil || len(qos.Limits.Max.TRES.Per.Node) != 1 {
		t.Fatal("expected 1 per-node TRES entry")
	}
	if qos.Limits.Max.TRES.Per.Node[0].Name != "gpu" {
		t.Errorf("expected gres/gpu, got %q", qos.Limits.Max.TRES.Per.Node[0].Name)
	}
}

func TestModelToAPI_GrpTRES(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.GrpTRES = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 512},
		{Type: "gres", Name: "gpu", Count: 32},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits == nil || qos.Limits.Max == nil || qos.Limits.Max.TRES == nil {
		t.Fatal("expected TRES limits to be set")
	}
	if len(qos.Limits.Max.TRES.Total) != 2 {
		t.Errorf("expected 2 grp_tres entries, got %d", len(qos.Limits.Max.TRES.Total))
	}
}

func TestModelToAPI_GrpTRESMins(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.GrpTRESMins = buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 460800},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits.Max.TRES.Minutes == nil || len(qos.Limits.Max.TRES.Minutes.Total) != 1 {
		t.Fatal("expected 1 grp_tres_mins entry")
	}
}

func TestModelToAPI_MaxTRESPerUser(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.MaxTRESPerUser = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 256},
	})
	m.MaxTRESMinsPerUser = buildTRESSet(t, []client.TRES{
		{Type: "cpu", Count: 2880000},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits.Max.TRES.Per.User == nil || len(qos.Limits.Max.TRES.Per.User) != 1 {
		t.Fatal("expected 1 max_tres_per_user entry")
	}
	if qos.Limits.Max.TRES.Minutes.Per.User == nil || len(qos.Limits.Max.TRES.Minutes.Per.User) != 1 {
		t.Fatal("expected 1 max_tres_mins_per_user entry")
	}
}

func TestModelToAPI_MaxTRESPerAccount(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.MaxTRESPerAccount = buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 64},
	})
	m.MaxTRESMinsPerAccount = buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 230400},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits.Max.TRES.Per.Account == nil || len(qos.Limits.Max.TRES.Per.Account) != 1 {
		t.Fatal("expected 1 max_tres_per_account entry")
	}
	if qos.Limits.Max.TRES.Minutes.Per.Account == nil || len(qos.Limits.Max.TRES.Minutes.Per.Account) != 1 {
		t.Fatal("expected 1 max_tres_mins_per_account entry")
	}
}

func TestModelToAPI_MinTRESPerJob(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.MinTRESPerJob = buildTRESSet(t, []client.TRES{
		{Type: "gres", Name: "gpu", Count: 1},
	})
	qos := r.modelToAPI(ctx, m)

	if qos.Limits == nil || qos.Limits.Min == nil || qos.Limits.Min.TRES == nil ||
		qos.Limits.Min.TRES.Per == nil || len(qos.Limits.Min.TRES.Per.Job) != 1 {
		t.Fatal("expected 1 min_tres_per_job entry")
	}
	if qos.Limits.Min.TRES.Per.Job[0].Name != "gpu" {
		t.Errorf("expected gres/gpu in min_tres_per_job, got %q", qos.Limits.Min.TRES.Per.Job[0].Name)
	}
}

func TestModelToAPI_JobCounts(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	m := emptyModel("testqos")
	m.GrpJobs = types.Int64Value(500)
	m.GrpSubmitJobs = types.Int64Value(2000)
	m.MaxJobsPerUser = types.Int64Value(50)
	m.MaxSubmitJobsPerUser = types.Int64Value(200)
	m.MaxJobsPerAccount = types.Int64Value(100)
	m.MaxSubmitJobsPerAccount = types.Int64Value(400)
	m.GraceTime = types.Int64Value(120)
	qos := r.modelToAPI(ctx, m)

	if qos.Limits == nil {
		t.Fatal("expected limits to be set")
	}
	if qos.Limits.GraceTime != 120 {
		t.Errorf("expected grace_time 120, got %d", qos.Limits.GraceTime)
	}

	max := qos.Limits.Max
	if max == nil || max.Jobs == nil {
		t.Fatal("expected max.jobs to be set")
	}
	if max.Jobs.Count == nil || max.Jobs.Count.Number != 500 {
		t.Errorf("expected grp_jobs 500, got %v", max.Jobs.Count)
	}
	if max.Jobs.Per == nil || max.Jobs.Per.User == nil || max.Jobs.Per.User.Number != 50 {
		t.Errorf("expected max_jobs_per_user 50, got %v", max.Jobs.Per)
	}
	if max.Jobs.Per.Account == nil || max.Jobs.Per.Account.Number != 100 {
		t.Errorf("expected max_jobs_per_account 100, got %v", max.Jobs.Per.Account)
	}
	if max.Jobs.ActiveJobs == nil || max.Jobs.ActiveJobs.Per == nil {
		t.Fatal("expected max.jobs.active_jobs to be set")
	}
	if max.Jobs.ActiveJobs.Per.User == nil || max.Jobs.ActiveJobs.Per.User.Number != 200 {
		t.Errorf("expected max_submit_jobs_per_user 200, got %v", max.Jobs.ActiveJobs.Per.User)
	}
	if max.Jobs.ActiveJobs.Per.Account == nil || max.Jobs.ActiveJobs.Per.Account.Number != 400 {
		t.Errorf("expected max_submit_jobs_per_account 400, got %v", max.Jobs.ActiveJobs.Per.Account)
	}
	if max.ActiveJobs == nil || max.ActiveJobs.Count == nil || max.ActiveJobs.Count.Number != 2000 {
		t.Errorf("expected grp_submit_jobs 2000, got %v", max.ActiveJobs)
	}
}

func TestModelToAPI_NoLimitsWhenAllNull(t *testing.T) {
	r := &qosResource{}
	ctx := context.Background()

	// A model with only name — no limit fields set — must not populate Limits.
	m := emptyModel("testqos")
	m.Priority = types.Int64Value(100) // priority is not a limit field
	qos := r.modelToAPI(ctx, m)

	if qos.Limits != nil {
		t.Error("expected nil Limits when no limit fields are set")
	}
}
