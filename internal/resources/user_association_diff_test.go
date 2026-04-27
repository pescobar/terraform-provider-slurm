package resources

import (
	"testing"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// helper to create a *SlurmInt
func slurmInt(n int) *client.SlurmInt {
	return &client.SlurmInt{Number: n, Set: true}
}

// helper to create a *int for SharesRaw
func intPtr(n int) *int {
	return &n
}

// helper to create an association with common fields
func makeAssoc(account, partition string, fairshare int, defaultQOS string, qos []string) client.Association {
	a := client.Association{
		Account:   account,
		Partition: partition,
		Cluster:   "linux",
		User:      "testuser",
	}
	if fairshare > 0 {
		a.SharesRaw = intPtr(fairshare)
	}
	if defaultQOS != "" {
		a.Default = &client.AssociationDefaults{QOS: defaultQOS}
	}
	if len(qos) > 0 {
		a.QOS = qos
	}
	return a
}

func TestDiffAssociations_NoChanges(t *testing.T) {
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal", "high"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal", "high"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}

	diff := DiffAssociations(old, new)

	if !diff.IsEmpty() {
		t.Errorf("Expected no changes, got: create=%d update=%d delete=%d",
			len(diff.Create), len(diff.Update), len(diff.Delete))
	}
}

func TestDiffAssociations_NoChanges_QOSOrderDifferent(t *testing.T) {
	// QOS lists with same elements but different order should be considered equal
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"high", "normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal", "high"}),
	}

	diff := DiffAssociations(old, new)

	if !diff.IsEmpty() {
		t.Errorf("Expected no changes when QOS order differs, got: create=%d update=%d delete=%d",
			len(diff.Create), len(diff.Update), len(diff.Delete))
	}
}

func TestDiffAssociations_CreateOnly(t *testing.T) {
	old := []client.Association{}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 2 {
		t.Fatalf("Expected 2 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 0 {
		t.Errorf("Expected 0 updates, got %d", len(diff.Update))
	}
	if len(diff.Delete) != 0 {
		t.Errorf("Expected 0 deletes, got %d", len(diff.Delete))
	}

	// Verify sorted order (chemistry before physics)
	if diff.Create[0].Account != "chemistry" {
		t.Errorf("Expected first create to be 'chemistry', got '%s'", diff.Create[0].Account)
	}
	if diff.Create[1].Account != "physics" {
		t.Errorf("Expected second create to be 'physics', got '%s'", diff.Create[1].Account)
	}
}

func TestDiffAssociations_DeleteOnly(t *testing.T) {
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}
	new := []client.Association{}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 0 {
		t.Errorf("Expected 0 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 0 {
		t.Errorf("Expected 0 updates, got %d", len(diff.Update))
	}
	if len(diff.Delete) != 2 {
		t.Fatalf("Expected 2 deletes, got %d", len(diff.Delete))
	}

	// Verify sorted order
	if diff.Delete[0].Account != "chemistry" {
		t.Errorf("Expected first delete to be 'chemistry', got '%s'", diff.Delete[0].Account)
	}
}

func TestDiffAssociations_UpdateFairshare(t *testing.T) {
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 200, "normal", []string{"normal"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 0 {
		t.Errorf("Expected 0 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update, got %d", len(diff.Update))
	}
	if len(diff.Delete) != 0 {
		t.Errorf("Expected 0 deletes, got %d", len(diff.Delete))
	}
	if *diff.Update[0].SharesRaw != 200 {
		t.Errorf("Expected updated fairshare=200, got %d", *diff.Update[0].SharesRaw)
	}
}

func TestDiffAssociations_UpdateDefaultQOS(t *testing.T) {
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal", "high"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "high", []string{"normal", "high"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update, got %d", len(diff.Update))
	}
	if diff.Update[0].Default.QOS != "high" {
		t.Errorf("Expected updated default QOS='high', got '%s'", diff.Update[0].Default.QOS)
	}
}

func TestDiffAssociations_UpdateQOSList(t *testing.T) {
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal", "high"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update, got %d", len(diff.Update))
	}
	if len(diff.Update[0].QOS) != 2 {
		t.Errorf("Expected updated QOS list with 2 entries, got %d", len(diff.Update[0].QOS))
	}
}

func TestDiffAssociations_MixedOperations(t *testing.T) {
	// This is the most realistic scenario:
	// - physics: fairshare changes (update)
	// - chemistry: removed (delete)
	// - shared: added (create)
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 200, "normal", []string{"normal"}),
		makeAssoc("shared", "", 30, "lowprio", []string{"normal", "lowprio"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 1 {
		t.Fatalf("Expected 1 create, got %d", len(diff.Create))
	}
	if diff.Create[0].Account != "shared" {
		t.Errorf("Expected create for 'shared', got '%s'", diff.Create[0].Account)
	}

	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update, got %d", len(diff.Update))
	}
	if diff.Update[0].Account != "physics" {
		t.Errorf("Expected update for 'physics', got '%s'", diff.Update[0].Account)
	}
	if *diff.Update[0].SharesRaw != 200 {
		t.Errorf("Expected updated fairshare=200, got %d", *diff.Update[0].SharesRaw)
	}

	if len(diff.Delete) != 1 {
		t.Fatalf("Expected 1 delete, got %d", len(diff.Delete))
	}
	if diff.Delete[0].Account != "chemistry" {
		t.Errorf("Expected delete for 'chemistry', got '%s'", diff.Delete[0].Account)
	}
}

func TestDiffAssociations_WithPartitions(t *testing.T) {
	// Same account but different partitions are different associations
	old := []client.Association{
		makeAssoc("physics", "gpu", 100, "normal", []string{"normal"}),
		makeAssoc("physics", "batch", 50, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "gpu", 100, "normal", []string{"normal"}),
		// batch partition removed, debug added
		makeAssoc("physics", "debug", 30, "normal", []string{"normal"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 1 {
		t.Fatalf("Expected 1 create, got %d", len(diff.Create))
	}
	if diff.Create[0].Partition != "debug" {
		t.Errorf("Expected create for partition 'debug', got '%s'", diff.Create[0].Partition)
	}

	if len(diff.Update) != 0 {
		t.Errorf("Expected 0 updates, got %d", len(diff.Update))
	}

	if len(diff.Delete) != 1 {
		t.Fatalf("Expected 1 delete, got %d", len(diff.Delete))
	}
	if diff.Delete[0].Partition != "batch" {
		t.Errorf("Expected delete for partition 'batch', got '%s'", diff.Delete[0].Partition)
	}
}

func TestDiffAssociations_NilToValue(t *testing.T) {
	// Old association has no fairshare set, new one does
	old := []client.Association{
		{Account: "physics", Cluster: "linux", User: "testuser"},
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update when going from nil to value, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_ValueToNil(t *testing.T) {
	// Old association has fairshare, new one doesn't
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
	}
	new := []client.Association{
		{Account: "physics", Cluster: "linux", User: "testuser"},
	}

	diff := DiffAssociations(old, new)

	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update when going from value to nil, got %d", len(diff.Update))
	}
}

// Renamed: Per.Count is GrpJobs in v0.0.42 (MaxJobs lives at Jobs.Active).
func TestDiffAssociations_GrpJobsChange(t *testing.T) {
	old := []client.Association{
		{
			Account:   "physics",
			Cluster:   "linux",
			User:      "testuser",
			SharesRaw: intPtr(100),
			Max: &client.AssociationMax{
				Jobs: &client.AssociationMaxJobs{
					Per: &client.AssociationMaxJobsPer{Count: slurmInt(50)},
				},
			},
		},
	}
	new := []client.Association{
		{
			Account:   "physics",
			Cluster:   "linux",
			User:      "testuser",
			SharesRaw: intPtr(100),
			Max: &client.AssociationMax{
				Jobs: &client.AssociationMaxJobs{
					Per: &client.AssociationMaxJobsPer{Count: slurmInt(100)},
				},
			},
		},
	}
	diff := DiffAssociations(old, new)
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_jobs change, got %d", len(diff.Update))
	}
}

// ---------------------------------------------------------------------------
// New-field diff tests (all fields added in the association limits expansion)
// ---------------------------------------------------------------------------

func base() client.Association {
	return client.Association{Account: "acct", Cluster: "linux", User: "u"}
}

func TestDiffAssociations_MaxJobsChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Active: slurmInt(5)}}
	n.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Active: slurmInt(10)}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_jobs (Active) change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_MaxJobsAccrueChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Accruing: slurmInt(10)}}
	n.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Accruing: slurmInt(20)}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_jobs_accrue change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_MaxSubmitJobsChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Total: slurmInt(20)}}
	n.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Total: slurmInt(40)}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_submit_jobs change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_MaxWallPJChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Per: &client.AssociationMaxJobsPer{WallClock: slurmInt(60)}}}
	n.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Per: &client.AssociationMaxJobsPer{WallClock: slurmInt(120)}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_wall_pj change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_GrpJobsAccrueChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Per: &client.AssociationMaxJobsPer{Accruing: slurmInt(200)}}}
	n.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Per: &client.AssociationMaxJobsPer{Accruing: slurmInt(400)}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_jobs_accrue change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_GrpSubmitJobsChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Per: &client.AssociationMaxJobsPer{Submitted: slurmInt(400)}}}
	n.Max = &client.AssociationMax{Jobs: &client.AssociationMaxJobs{Per: &client.AssociationMaxJobsPer{Submitted: slurmInt(800)}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_submit_jobs change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_GrpWallChange(t *testing.T) {
	o, n := base(), base()
	o.Max = &client.AssociationMax{Per: &client.AssociationMaxPerNode{Account: &client.AssociationMaxPerAccount{WallClock: slurmInt(1440)}}}
	n.Max = &client.AssociationMax{Per: &client.AssociationMaxPerNode{Account: &client.AssociationMaxPerAccount{WallClock: slurmInt(2880)}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_wall change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_PriorityChange(t *testing.T) {
	o, n := base(), base()
	o.Priority = slurmInt(10)
	n.Priority = slurmInt(20)
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for priority change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_MaxTRESPerJobChange(t *testing.T) {
	cpu8 := []client.TRES{{Type: "cpu", Count: 8}}
	cpu16 := []client.TRES{{Type: "cpu", Count: 16}}
	o, n := base(), base()
	o.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Per: &client.AssociationMaxTRESPer{Job: cpu8}}}
	n.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Per: &client.AssociationMaxTRESPer{Job: cpu16}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_tres_per_job change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_MaxTRESPerNodeChange(t *testing.T) {
	cpu4 := []client.TRES{{Type: "cpu", Count: 4}}
	cpu8 := []client.TRES{{Type: "cpu", Count: 8}}
	o, n := base(), base()
	o.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Per: &client.AssociationMaxTRESPer{Node: cpu4}}}
	n.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Per: &client.AssociationMaxTRESPer{Node: cpu8}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_tres_per_node change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_MaxTRESMinsPerJobChange(t *testing.T) {
	cpu480 := []client.TRES{{Type: "cpu", Count: 480}}
	cpu960 := []client.TRES{{Type: "cpu", Count: 960}}
	o, n := base(), base()
	o.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{
		Minutes: &client.AssociationMaxTRESMins{Per: &client.AssociationMaxTRESMinsPer{Job: cpu480}},
	}}
	n.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{
		Minutes: &client.AssociationMaxTRESMins{Per: &client.AssociationMaxTRESMinsPer{Job: cpu960}},
	}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_tres_mins_per_job change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_GrpTRESChange(t *testing.T) {
	cpu256 := []client.TRES{{Type: "cpu", Count: 256}}
	cpu512 := []client.TRES{{Type: "cpu", Count: 512}}
	o, n := base(), base()
	o.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Total: cpu256}}
	n.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Total: cpu512}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_tres change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_GrpTRESMinsChange(t *testing.T) {
	cpu153600 := []client.TRES{{Type: "cpu", Count: 153600}}
	cpu307200 := []client.TRES{{Type: "cpu", Count: 307200}}
	o, n := base(), base()
	o.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Group: &client.AssociationMaxTRESGroup{Minutes: cpu153600}}}
	n.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Group: &client.AssociationMaxTRESGroup{Minutes: cpu307200}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_tres_mins change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_GrpTRESRunMinsChange(t *testing.T) {
	cpu76800 := []client.TRES{{Type: "cpu", Count: 76800}}
	cpu153600 := []client.TRES{{Type: "cpu", Count: 153600}}
	o, n := base(), base()
	o.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Group: &client.AssociationMaxTRESGroup{Active: cpu76800}}}
	n.Max = &client.AssociationMax{TRES: &client.AssociationMaxTRES{Group: &client.AssociationMaxTRESGroup{Active: cpu153600}}}
	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for grp_tres_run_mins change, got %d", len(diff.Update))
	}
}

func TestDiffAssociations_AllNewLimitsNoChange(t *testing.T) {
	full := func() client.Association {
		a := base()
		a.Priority = slurmInt(10)
		a.Max = &client.AssociationMax{
			Jobs: &client.AssociationMaxJobs{
				Per: &client.AssociationMaxJobsPer{
					Count:     slurmInt(100),
					Accruing:  slurmInt(200),
					Submitted: slurmInt(400),
					WallClock: slurmInt(60),
				},
				Active:   slurmInt(5),
				Accruing: slurmInt(10),
				Total:    slurmInt(20),
			},
			TRES: &client.AssociationMaxTRES{
				Total: []client.TRES{{Type: "cpu", Count: 256}},
				Group: &client.AssociationMaxTRESGroup{
					Minutes: []client.TRES{{Type: "cpu", Count: 153600}},
					Active:  []client.TRES{{Type: "cpu", Count: 76800}},
				},
				Minutes: &client.AssociationMaxTRESMins{
					Per: &client.AssociationMaxTRESMinsPer{Job: []client.TRES{{Type: "cpu", Count: 480}}},
				},
				Per: &client.AssociationMaxTRESPer{
					Job:  []client.TRES{{Type: "cpu", Count: 8}, {Type: "mem", Count: 16384}},
					Node: []client.TRES{{Type: "cpu", Count: 4}},
				},
			},
			Per: &client.AssociationMaxPerNode{
				Account: &client.AssociationMaxPerAccount{WallClock: slurmInt(1440)},
			},
		}
		return a
	}
	diff := DiffAssociations([]client.Association{full()}, []client.Association{full()})
	if !diff.IsEmpty() {
		t.Errorf("Expected no diff when all limits are identical, got: create=%d update=%d delete=%d",
			len(diff.Create), len(diff.Update), len(diff.Delete))
	}
}

func TestDiffAssociations_AllNewLimitsChanged(t *testing.T) {
	o := base()
	o.Priority = slurmInt(10)
	o.Max = &client.AssociationMax{
		Jobs: &client.AssociationMaxJobs{
			Active:   slurmInt(5),
			Accruing: slurmInt(10),
			Total:    slurmInt(20),
			Per: &client.AssociationMaxJobsPer{
				Count:     slurmInt(100),
				Accruing:  slurmInt(200),
				Submitted: slurmInt(400),
				WallClock: slurmInt(60),
			},
		},
		TRES: &client.AssociationMaxTRES{
			Total: []client.TRES{{Type: "cpu", Count: 256}},
			Group: &client.AssociationMaxTRESGroup{
				Minutes: []client.TRES{{Type: "cpu", Count: 153600}},
				Active:  []client.TRES{{Type: "cpu", Count: 76800}},
			},
			Per:     &client.AssociationMaxTRESPer{Job: []client.TRES{{Type: "cpu", Count: 8}}},
			Minutes: &client.AssociationMaxTRESMins{Per: &client.AssociationMaxTRESMinsPer{Job: []client.TRES{{Type: "cpu", Count: 480}}}},
		},
		Per: &client.AssociationMaxPerNode{Account: &client.AssociationMaxPerAccount{WallClock: slurmInt(1440)}},
	}

	n := base()
	n.Priority = slurmInt(20) // changed
	n.Max = &client.AssociationMax{
		Jobs: &client.AssociationMaxJobs{
			Active:   slurmInt(10),  // changed
			Accruing: slurmInt(20),  // changed
			Total:    slurmInt(40),  // changed
			Per: &client.AssociationMaxJobsPer{
				Count:     slurmInt(200),  // changed
				Accruing:  slurmInt(400),  // changed
				Submitted: slurmInt(800),  // changed
				WallClock: slurmInt(120),  // changed
			},
		},
		TRES: &client.AssociationMaxTRES{
			Total: []client.TRES{{Type: "cpu", Count: 512}}, // changed
			Group: &client.AssociationMaxTRESGroup{
				Minutes: []client.TRES{{Type: "cpu", Count: 307200}}, // changed
				Active:  []client.TRES{{Type: "cpu", Count: 153600}}, // changed
			},
			Per:     &client.AssociationMaxTRESPer{Job: []client.TRES{{Type: "cpu", Count: 16}}}, // changed
			Minutes: &client.AssociationMaxTRESMins{Per: &client.AssociationMaxTRESMinsPer{Job: []client.TRES{{Type: "cpu", Count: 960}}}}, // changed
		},
		Per: &client.AssociationMaxPerNode{Account: &client.AssociationMaxPerAccount{WallClock: slurmInt(2880)}}, // changed
	}

	diff := DiffAssociations([]client.Association{o}, []client.Association{n})
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update when all limits change, got %d", len(diff.Update))
	}
	if len(diff.Create) != 0 || len(diff.Delete) != 0 {
		t.Errorf("Expected no creates/deletes, got create=%d delete=%d", len(diff.Create), len(diff.Delete))
	}
}

// ---------------------------------------------------------------------------
// tresSlicesEqual tests
// ---------------------------------------------------------------------------

func TestTresSlicesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []client.TRES
		expected bool
	}{
		{
			name:     "both nil",
			expected: true,
		},
		{
			name:     "both empty",
			a:        []client.TRES{},
			b:        []client.TRES{},
			expected: true,
		},
		{
			name:     "identical single",
			a:        []client.TRES{{Type: "cpu", Count: 8}},
			b:        []client.TRES{{Type: "cpu", Count: 8}},
			expected: true,
		},
		{
			name:     "different count",
			a:        []client.TRES{{Type: "cpu", Count: 8}},
			b:        []client.TRES{{Type: "cpu", Count: 16}},
			expected: false,
		},
		{
			name:     "different type",
			a:        []client.TRES{{Type: "cpu", Count: 8}},
			b:        []client.TRES{{Type: "mem", Count: 8}},
			expected: false,
		},
		{
			name:     "different length",
			a:        []client.TRES{{Type: "cpu", Count: 8}},
			b:        []client.TRES{{Type: "cpu", Count: 8}, {Type: "mem", Count: 1024}},
			expected: false,
		},
		{
			name: "order-insensitive multi",
			a:    []client.TRES{{Type: "cpu", Count: 8}, {Type: "mem", Count: 1024}},
			b:    []client.TRES{{Type: "mem", Count: 1024}, {Type: "cpu", Count: 8}},
			expected: true,
		},
		{
			name:     "gres/gpu same",
			a:        []client.TRES{{Type: "gres", Name: "gpu", Count: 4}},
			b:        []client.TRES{{Type: "gres", Name: "gpu", Count: 4}},
			expected: true,
		},
		{
			name:     "gres/gpu different count",
			a:        []client.TRES{{Type: "gres", Name: "gpu", Count: 4}},
			b:        []client.TRES{{Type: "gres", Name: "gpu", Count: 8}},
			expected: false,
		},
		{
			name:     "nil vs empty",
			a:        nil,
			b:        []client.TRES{},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tresSlicesEqual(tc.a, tc.b)
			if got != tc.expected {
				t.Errorf("tresSlicesEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.expected)
			}
		})
	}
}

func TestDiffAssociations_CompleteReplacement(t *testing.T) {
	// All old associations are removed, all new ones are different
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("engineering", "", 80, "normal", []string{"normal"}),
		makeAssoc("shared", "", 30, "lowprio", []string{"lowprio"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 2 {
		t.Errorf("Expected 2 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 0 {
		t.Errorf("Expected 0 updates, got %d", len(diff.Update))
	}
	if len(diff.Delete) != 2 {
		t.Errorf("Expected 2 deletes, got %d", len(diff.Delete))
	}
}

func TestDiffAssociations_SingleToMany(t *testing.T) {
	// User goes from single account to multi-account (common scenario)
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
		makeAssoc("shared", "", 30, "lowprio", []string{"normal", "lowprio"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 2 {
		t.Fatalf("Expected 2 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 0 {
		t.Errorf("Expected 0 updates (physics unchanged), got %d", len(diff.Update))
	}
	if len(diff.Delete) != 0 {
		t.Errorf("Expected 0 deletes, got %d", len(diff.Delete))
	}
}

func TestDiffAssociations_ManyToSingle(t *testing.T) {
	// User goes from multi-account back to single account
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
		makeAssoc("shared", "", 30, "lowprio", []string{"normal", "lowprio"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 0 {
		t.Errorf("Expected 0 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 0 {
		t.Errorf("Expected 0 updates, got %d", len(diff.Update))
	}
	if len(diff.Delete) != 2 {
		t.Fatalf("Expected 2 deletes, got %d", len(diff.Delete))
	}
}

func TestDiffAssociations_ChangeDefaultAccountAssociation(t *testing.T) {
	// The default account's association changes — this is the edge case
	// where both default_account and the association update in same apply
	old := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 50, "normal", []string{"normal"}),
	}
	new := []client.Association{
		makeAssoc("physics", "", 100, "normal", []string{"normal"}),
		makeAssoc("chemistry", "", 80, "high", []string{"normal", "high"}),
	}

	diff := DiffAssociations(old, new)

	if len(diff.Create) != 0 {
		t.Errorf("Expected 0 creates, got %d", len(diff.Create))
	}
	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update (chemistry changed), got %d", len(diff.Update))
	}
	if diff.Update[0].Account != "chemistry" {
		t.Errorf("Expected update for 'chemistry', got '%s'", diff.Update[0].Account)
	}
	if *diff.Update[0].SharesRaw != 80 {
		t.Errorf("Expected fairshare=80, got %d", *diff.Update[0].SharesRaw)
	}
	if diff.Update[0].Default.QOS != "high" {
		t.Errorf("Expected default QOS='high', got '%s'", diff.Update[0].Default.QOS)
	}
	if len(diff.Delete) != 0 {
		t.Errorf("Expected 0 deletes, got %d", len(diff.Delete))
	}
}

func TestAssociationKey_String(t *testing.T) {
	tests := []struct {
		key      AssociationKey
		expected string
	}{
		{AssociationKey{Account: "physics"}, "physics"},
		{AssociationKey{Account: "physics", Partition: "gpu"}, "physics/gpu"},
		{AssociationKey{Account: "physics", Partition: ""}, "physics"},
	}

	for _, tc := range tests {
		got := tc.key.String()
		if got != tc.expected {
			t.Errorf("AssociationKey{%q, %q}.String() = %q, want %q",
				tc.key.Account, tc.key.Partition, got, tc.expected)
		}
	}
}

func TestStringSlicesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []string
		expected bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different order", []string{"b", "a"}, []string{"a", "b"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different values", []string{"a", "b"}, []string{"a", "c"}, false},
		{"nil vs empty", nil, []string{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stringSlicesEqual(tc.a, tc.b)
			if got != tc.expected {
				t.Errorf("stringSlicesEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.expected)
			}
		})
	}
}
