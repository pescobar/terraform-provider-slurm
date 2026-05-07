package resources

import (
	"testing"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

func sint(n int) *client.SlurmInt { return &client.SlurmInt{Number: n, Set: true} }

func TestDsAssocJobsScalars_AllNullWhenMaxNil(t *testing.T) {
	out := dsAssocJobsScalars(client.Association{})
	if !out.maxJobs.IsNull() || !out.grpJobs.IsNull() || !out.maxWallPJ.IsNull() || !out.grpWall.IsNull() {
		t.Errorf("expected all-null when Max is nil, got %+v", out)
	}
}

func TestDsAssocJobsScalars_PerJobLimits(t *testing.T) {
	a := client.Association{
		Max: &client.AssociationMax{
			Jobs: &client.AssociationMaxJobs{
				Active:   sint(10),
				Accruing: sint(20),
				Total:    sint(30),
				Per: &client.AssociationMaxJobsPer{
					WallClock: sint(60),
				},
			},
		},
	}
	out := dsAssocJobsScalars(a)
	if v := out.maxJobs.ValueInt64(); v != 10 {
		t.Errorf("max_jobs = %d, want 10", v)
	}
	if v := out.maxJobsAccrue.ValueInt64(); v != 20 {
		t.Errorf("max_jobs_accrue = %d, want 20", v)
	}
	if v := out.maxSubmitJobs.ValueInt64(); v != 30 {
		t.Errorf("max_submit_jobs = %d, want 30", v)
	}
	if v := out.maxWallPJ.ValueInt64(); v != 60 {
		t.Errorf("max_wall_pj = %d, want 60", v)
	}
	// Group fields should still be null — only per-job set.
	if !out.grpJobs.IsNull() || !out.grpWall.IsNull() {
		t.Errorf("grp_* should be null, got grpJobs=%v grpWall=%v", out.grpJobs, out.grpWall)
	}
}

func TestDsAssocJobsScalars_GroupLimits(t *testing.T) {
	a := client.Association{
		Max: &client.AssociationMax{
			Jobs: &client.AssociationMaxJobs{
				Per: &client.AssociationMaxJobsPer{
					Count:     sint(100),
					Accruing:  sint(200),
					Submitted: sint(300),
				},
			},
			Per: &client.AssociationMaxPerNode{
				Account: &client.AssociationMaxPerAccount{
					WallClock: sint(2880),
				},
			},
		},
	}
	out := dsAssocJobsScalars(a)
	if v := out.grpJobs.ValueInt64(); v != 100 {
		t.Errorf("grp_jobs = %d, want 100", v)
	}
	if v := out.grpJobsAccrue.ValueInt64(); v != 200 {
		t.Errorf("grp_jobs_accrue = %d, want 200", v)
	}
	if v := out.grpSubmitJobs.ValueInt64(); v != 300 {
		t.Errorf("grp_submit_jobs = %d, want 300", v)
	}
	if v := out.grpWall.ValueInt64(); v != 2880 {
		t.Errorf("grp_wall = %d, want 2880", v)
	}
}

func TestDsAssocJobsScalars_UnsetSlurmInt_StaysNull(t *testing.T) {
	// SlurmInt with Set=false must not contribute a value.
	a := client.Association{
		Max: &client.AssociationMax{
			Jobs: &client.AssociationMaxJobs{
				Active: &client.SlurmInt{Number: 5, Set: false},
			},
		},
	}
	out := dsAssocJobsScalars(a)
	if !out.maxJobs.IsNull() {
		t.Errorf("max_jobs should be null when Set=false, got %v", out.maxJobs)
	}
}
