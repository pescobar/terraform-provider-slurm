package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Partition endpoints live under /slurm/ (slurmctld), not /slurmdb/ (slurmdbd).
// GET support exists in every API version this provider targets (v0.0.42+).
//
// The provider deliberately exposes partitions as a DATA SOURCE only. Slurm
// 26.05 (v0.0.45) added POST/DELETE support, but partitions created through
// the REST API live only in slurmctld's runtime state — they are NOT written
// to slurm.conf and are silently lost on slurmctld restart (verified
// empirically against 26.05.1). A Terraform resource would therefore show
// every managed partition as deleted after any controller restart. Revisit if
// SchedMD makes REST-created partitions persistent.

// PartitionResponse is the response from GET /slurm/{version}/partition(s).
type PartitionResponse struct {
	Partitions []Partition    `json:"partitions"`
	Errors     []SlurmError   `json:"errors"`
	Warnings   []SlurmWarning `json:"warnings"`
}

// Partition represents a Slurm partition as returned by slurmctld.
// Only the fields surfaced by the data source are modelled.
type Partition struct {
	Name        string              `json:"name"`
	Alternate   string              `json:"alternate,omitempty"`
	NodeSets    string              `json:"node_sets,omitempty"`
	Flags       []string            `json:"flags,omitempty"`
	PreemptMode []string            `json:"preempt_mode,omitempty"`
	GraceTime   int64               `json:"grace_time,omitempty"`
	Nodes       *PartitionNodes     `json:"nodes,omitempty"`
	Accounts    *PartitionAccounts  `json:"accounts,omitempty"`
	Groups      *PartitionGroups    `json:"groups,omitempty"`
	QOS         *PartitionQOS       `json:"qos,omitempty"`
	CPUs        *PartitionCPUs      `json:"cpus,omitempty"`
	Defaults    *PartitionDefaults  `json:"defaults,omitempty"`
	Maximums    *PartitionMaximums  `json:"maximums,omitempty"`
	Priority    *PartitionPriority  `json:"priority,omitempty"`
	Partition   *PartitionSubStatus `json:"partition,omitempty"`
}

// PartitionNodes holds the node membership of a partition.
type PartitionNodes struct {
	AllowedAllocation string `json:"allowed_allocation,omitempty"`
	Configured        string `json:"configured,omitempty"`
	Total             int64  `json:"total,omitempty"`
}

// PartitionAccounts holds the account ACLs of a partition.
type PartitionAccounts struct {
	Allowed string `json:"allowed,omitempty"`
	Deny    string `json:"deny,omitempty"`
}

// PartitionGroups holds the group ACL of a partition.
type PartitionGroups struct {
	Allowed string `json:"allowed,omitempty"`
}

// PartitionQOS holds the QOS ACLs and the partition-assigned QOS.
type PartitionQOS struct {
	Allowed  string `json:"allowed,omitempty"`
	Deny     string `json:"deny,omitempty"`
	Assigned string `json:"assigned,omitempty"`
}

// PartitionCPUs holds CPU totals for a partition.
type PartitionCPUs struct {
	Total int64 `json:"total,omitempty"`
}

// PartitionDefaults holds per-job defaults applied by a partition.
type PartitionDefaults struct {
	Time *SlurmInt `json:"time,omitempty"`
}

// PartitionMaximums holds per-job maximums enforced by a partition.
type PartitionMaximums struct {
	Time  *SlurmInt `json:"time,omitempty"`
	Nodes *SlurmInt `json:"nodes,omitempty"`
}

// PartitionPriority holds the scheduling priority knobs of a partition.
type PartitionPriority struct {
	JobFactor int64 `json:"job_factor,omitempty"`
	Tier      int64 `json:"tier,omitempty"`
}

// PartitionSubStatus holds the nested "partition" object (state et al.).
type PartitionSubStatus struct {
	State []string `json:"state,omitempty"`
}

// GetPartition returns a single partition by name, or nil when it does not
// exist. slurmctld reports an unknown partition as HTTP 200 with an empty
// partitions list and only a warning (verified on v0.0.42 and v0.0.45).
func (c *Client) GetPartition(ctx context.Context, name string) (*Partition, error) {
	path := c.slurmPath(fmt.Sprintf("partition/%s", url.PathEscape(name)))
	data, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp PartitionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal partition response: %w", err)
	}
	if len(resp.Partitions) == 0 {
		return nil, nil // not found
	}
	return &resp.Partitions[0], nil
}
