// Package resources contains Terraform resource implementations.
//
// This file contains the association diff logic used by the user resource
// to determine which associations need to be created, updated, or deleted
// when the user's embedded association blocks change.
//
// The logic is extracted into pure functions to make it independently testable
// without needing a running Slurm cluster or Terraform framework.
package resources

import (
	"fmt"
	"sort"

	"github.com/pescobar/terraform-provider-slurm/internal/client"
)

// AssociationKey uniquely identifies an association within a user.
// The combination of account + partition is the natural key, since a user
// can only have one association per (account, partition) pair on a given cluster.
type AssociationKey struct {
	Account   string
	Partition string
}

func (k AssociationKey) String() string {
	if k.Partition == "" {
		return k.Account
	}
	return fmt.Sprintf("%s/%s", k.Account, k.Partition)
}

// AssociationDiff describes the changes needed to reconcile the current
// (old) set of associations with the desired (new) set.
type AssociationDiff struct {
	// Create contains associations that exist in the new set but not in the old set.
	Create []client.Association

	// Update contains associations that exist in both sets but with changed attributes.
	Update []client.Association

	// Delete contains the keys of associations that exist in the old set but not in the new set.
	Delete []AssociationKey
}

// IsEmpty returns true if no changes are needed.
func (d *AssociationDiff) IsEmpty() bool {
	return len(d.Create) == 0 && len(d.Update) == 0 && len(d.Delete) == 0
}

// DiffAssociations computes the diff between old and new association sets.
//
// It compares associations by their natural key (account + partition) and
// determines which need to be created, updated, or deleted.
//
// This function is pure — it has no side effects and makes no API calls.
func DiffAssociations(oldAssocs, newAssocs []client.Association) AssociationDiff {
	oldMap := buildAssociationMap(oldAssocs)
	newMap := buildAssociationMap(newAssocs)

	var diff AssociationDiff

	// Find associations to create or update
	for key, newAssoc := range newMap {
		oldAssoc, exists := oldMap[key]
		if !exists {
			// New association — needs to be created
			diff.Create = append(diff.Create, newAssoc)
		} else if !associationsEqual(oldAssoc, newAssoc) {
			// Existing association with changed attributes — needs update
			diff.Update = append(diff.Update, newAssoc)
		}
		// If exists and equal, no action needed
	}

	// Find associations to delete
	for key := range oldMap {
		if _, exists := newMap[key]; !exists {
			diff.Delete = append(diff.Delete, key)
		}
	}

	// Sort for deterministic ordering (makes testing and logging predictable)
	sort.Slice(diff.Create, func(i, j int) bool {
		return associationSortKey(diff.Create[i]) < associationSortKey(diff.Create[j])
	})
	sort.Slice(diff.Update, func(i, j int) bool {
		return associationSortKey(diff.Update[i]) < associationSortKey(diff.Update[j])
	})
	sort.Slice(diff.Delete, func(i, j int) bool {
		return diff.Delete[i].String() < diff.Delete[j].String()
	})

	return diff
}

// buildAssociationMap indexes a slice of associations by their natural key.
func buildAssociationMap(assocs []client.Association) map[AssociationKey]client.Association {
	m := make(map[AssociationKey]client.Association, len(assocs))
	for _, a := range assocs {
		key := AssociationKey{
			Account:   a.Account,
			Partition: a.Partition,
		}
		m[key] = a
	}
	return m
}

// associationSortKey returns a string used for sorting associations deterministically.
func associationSortKey(a client.Association) string {
	return fmt.Sprintf("%s/%s", a.Account, a.Partition)
}

// associationsEqual compares two associations by their mutable attributes.
// It ignores the key fields (account, cluster, partition, user) since those
// are used for matching, not comparison.
func associationsEqual(a, b client.Association) bool {
	// Compare fairshare (shares_raw is a plain *int in the API)
	if !intPtrEqual(a.SharesRaw, b.SharesRaw) {
		return false
	}

	// Compare default QOS
	if !associationDefaultsEqual(a.Default, b.Default) {
		return false
	}

	// Compare QOS list
	if !stringSlicesEqual(a.QOS, b.QOS) {
		return false
	}

	// Compare max jobs
	if !associationMaxEqual(a.Max, b.Max) {
		return false
	}

	return true
}

// intPtrEqual compares two *int values.
func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// slurmIntEqual compares two *SlurmInt values.
func slurmIntEqual(a, b *client.SlurmInt) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Number == b.Number && a.Set == b.Set && a.Infinite == b.Infinite
}

// associationDefaultsEqual compares two *AssociationDefaults values.
func associationDefaultsEqual(a, b *client.AssociationDefaults) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.QOS == b.QOS
}

// associationMaxEqual compares two *AssociationMax values.
func associationMaxEqual(a, b *client.AssociationMax) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return associationMaxJobsEqual(a.Jobs, b.Jobs)
}

// associationMaxJobsEqual compares two *AssociationMaxJobs values.
func associationMaxJobsEqual(a, b *client.AssociationMaxJobs) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return associationMaxJobsPerEqual(a.Per, b.Per)
}

// associationMaxJobsPerEqual compares two *AssociationMaxJobsPer values.
func associationMaxJobsPerEqual(a, b *client.AssociationMaxJobsPer) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return slurmIntEqual(a.Count, b.Count)
}

// stringSlicesEqual compares two string slices, ignoring order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSorted := make([]string, len(a))
	bSorted := make([]string, len(b))
	copy(aSorted, a)
	copy(bSorted, b)
	sort.Strings(aSorted)
	sort.Strings(bSorted)
	for i := range aSorted {
		if aSorted[i] != bSorted[i] {
			return false
		}
	}
	return true
}
