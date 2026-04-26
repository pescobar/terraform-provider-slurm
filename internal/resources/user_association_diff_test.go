package resources

import (
	"testing"

	"github.com/pabloqc/terraform-provider-slurm/internal/client"
)

// helper to create a *SlurmInt
func slurmInt(n int) *client.SlurmInt {
	return &client.SlurmInt{Number: n, Set: true}
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
		a.Fairshare = slurmInt(fairshare)
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
	if diff.Update[0].Fairshare.Number != 200 {
		t.Errorf("Expected updated fairshare=200, got %d", diff.Update[0].Fairshare.Number)
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
	if diff.Update[0].Fairshare.Number != 200 {
		t.Errorf("Expected updated fairshare=200, got %d", diff.Update[0].Fairshare.Number)
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

func TestDiffAssociations_MaxJobsChange(t *testing.T) {
	old := []client.Association{
		{
			Account:   "physics",
			Cluster:   "linux",
			User:      "testuser",
			Fairshare: slurmInt(100),
			Max: &client.AssociationMax{
				Jobs: &client.AssociationMaxJobs{
					Per: &client.AssociationMaxJobsPer{
						Count: slurmInt(50),
					},
				},
			},
		},
	}
	new := []client.Association{
		{
			Account:   "physics",
			Cluster:   "linux",
			User:      "testuser",
			Fairshare: slurmInt(100),
			Max: &client.AssociationMax{
				Jobs: &client.AssociationMaxJobs{
					Per: &client.AssociationMaxJobsPer{
						Count: slurmInt(100),
					},
				},
			},
		},
	}

	diff := DiffAssociations(old, new)

	if len(diff.Update) != 1 {
		t.Fatalf("Expected 1 update for max_jobs change, got %d", len(diff.Update))
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
	if diff.Update[0].Fairshare.Number != 80 {
		t.Errorf("Expected fairshare=80, got %d", diff.Update[0].Fairshare.Number)
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
