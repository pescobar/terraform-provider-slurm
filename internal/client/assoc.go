package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// AssociationResponse is the response from GET /slurmdb/{version}/associations/
type AssociationResponse struct {
	Associations []Association  `json:"associations"`
	Errors       []SlurmError   `json:"errors"`
	Warnings     []SlurmWarning `json:"warnings"`
}

// Association represents a Slurm association (the link between user, account,
// cluster, and partition with attached limits).
// Field names match the v0.0.42 API schema for POST /associations/:
//   - shares_raw (plain int) is the fairshare value, not a SlurmInt object
//   - default.qos for default QOS, qos[] for allowed QOS list
type Association struct {
	Account   string               `json:"account,omitempty"`
	Cluster   string               `json:"cluster,omitempty"`
	Partition string               `json:"partition,omitempty"`
	User      string               `json:"user"`
	IsDefault bool                 `json:"is_default,omitempty"`
	Default   *AssociationDefaults `json:"default,omitempty"`
	SharesRaw *int                 `json:"shares_raw,omitempty"`
	Priority  *SlurmInt            `json:"priority,omitempty"`
	QOS       []string             `json:"qos,omitempty"`
	Max       *AssociationMax      `json:"max,omitempty"`
}

// AssociationDefaults contains default settings for an association.
type AssociationDefaults struct {
	QOS   string `json:"qos,omitempty"`
	WCKey string `json:"wckey,omitempty"`
}

// AssociationMax contains all limits for an association.
// The Slurm REST API (v0.0.42) folds both per-user ("Max*") and per-group
// ("Grp*") sacctmgr limits into this single "max" node.  There is no
// separate top-level "grp" key.
//
// Mapping from sacctmgr names → JSON paths:
//
//	MaxJobs        → max.jobs.active
//	MaxJobsAccrue  → max.jobs.accruing
//	MaxSubmitJobs  → max.jobs.total
//	MaxWall        → max.jobs.per.wall_clock  (minutes)
//	GrpJobs        → max.jobs.per.count
//	GrpJobsAccrue  → max.jobs.per.accruing
//	GrpSubmitJobs  → max.jobs.per.submitted
//	GrpWall        → max.per.account.wall_clock  (minutes)
//	MaxTRES        → max.tres.per.job
//	MaxTRESPerNode → max.tres.per.node
//	MaxTRESMins    → max.tres.minutes.per.job
//	GrpTRES        → max.tres.total
//	GrpTRESMins    → max.tres.group.minutes
//	GrpTRESRunMins → max.tres.group.active
type AssociationMax struct {
	Jobs *AssociationMaxJobs    `json:"jobs,omitempty"`
	TRES *AssociationMaxTRES    `json:"tres,omitempty"`
	Per  *AssociationMaxPerNode `json:"per,omitempty"` // holds GrpWall
}

// AssociationMaxJobs holds all job-count limits.
type AssociationMaxJobs struct {
	Per      *AssociationMaxJobsPer `json:"per,omitempty"`
	Active   *SlurmInt              `json:"active,omitempty"`   // MaxJobs
	Accruing *SlurmInt              `json:"accruing,omitempty"` // MaxJobsAccrue
	Total    *SlurmInt              `json:"total,omitempty"`    // MaxSubmitJobs
}

// AssociationMaxJobsPer holds the group-scoped job-count and wall-clock limits.
type AssociationMaxJobsPer struct {
	Count     *SlurmInt `json:"count,omitempty"`      // GrpJobs
	Accruing  *SlurmInt `json:"accruing,omitempty"`   // GrpJobsAccrue
	Submitted *SlurmInt `json:"submitted,omitempty"`  // GrpSubmitJobs
	WallClock *SlurmInt `json:"wall_clock,omitempty"` // MaxWall (per job, minutes)
}

// AssociationMaxTRES holds all TRES limits (both Max* and Grp* variants).
type AssociationMaxTRES struct {
	Total   []TRES                   `json:"total,omitempty"`   // GrpTRES
	Group   *AssociationMaxTRESGroup `json:"group,omitempty"`   // GrpTRESMins / GrpTRESRunMins
	Minutes *AssociationMaxTRESMins  `json:"minutes,omitempty"` // MaxTRESMins
	Per     *AssociationMaxTRESPer   `json:"per,omitempty"`     // MaxTRES / MaxTRESPerNode
}

// AssociationMaxTRESGroup holds group-aggregate TRES-minute limits.
type AssociationMaxTRESGroup struct {
	Minutes []TRES `json:"minutes,omitempty"` // GrpTRESMins
	Active  []TRES `json:"active,omitempty"`  // GrpTRESRunMins
}

// AssociationMaxTRESMins holds per-job TRES-minutes limits.
type AssociationMaxTRESMins struct {
	Total []TRES                     `json:"total,omitempty"`
	Per   *AssociationMaxTRESMinsPer `json:"per,omitempty"`
}

// AssociationMaxTRESMinsPer holds the per-job breakdown of TRES-minutes.
type AssociationMaxTRESMinsPer struct {
	Job []TRES `json:"job,omitempty"` // MaxTRESMins per job
}

// AssociationMaxTRESPer holds per-job and per-node TRES limits.
type AssociationMaxTRESPer struct {
	Job  []TRES `json:"job,omitempty"`  // MaxTRES per job
	Node []TRES `json:"node,omitempty"` // MaxTRESPerNode
}

// AssociationMaxPerNode is the "max.per" node; currently only carries GrpWall.
type AssociationMaxPerNode struct {
	Account *AssociationMaxPerAccount `json:"account,omitempty"`
}

// AssociationMaxPerAccount holds the per-account wall-clock limit (GrpWall).
type AssociationMaxPerAccount struct {
	WallClock *SlurmInt `json:"wall_clock,omitempty"` // GrpWall (minutes)
}

// GetAssociations returns all associations, optionally filtered by query params.
func (c *Client) GetAssociations(ctx context.Context, params map[string]string) (*AssociationResponse, error) {
	path := c.slurmdbPath("associations/")
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		path = fmt.Sprintf("%s?%s", path, q.Encode())
	}
	data, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp AssociationResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal associations response: %w", err)
	}
	return &resp, nil
}

// CreateAssociations creates or updates associations.
func (c *Client) CreateAssociations(ctx context.Context, associations []Association) error {
	body := map[string][]Association{
		"associations": associations,
	}
	_, err := c.doRequest(ctx, http.MethodPost, c.slurmdbPath("associations/"), body)
	return err
}

// DeleteAssociation deletes a single association by its key fields.
func (c *Client) DeleteAssociation(ctx context.Context, account, user, cluster, partition string) error {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	params := map[string]string{
		"account": account,
		"cluster": cluster,
	}
	if user != "" {
		params["user"] = user
	}
	if partition != "" {
		params["partition"] = partition
	}

	path := c.slurmdbPath("association/")
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	path = fmt.Sprintf("%s?%s", path, q.Encode())

	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}
