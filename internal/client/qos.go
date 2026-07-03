package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// QOSResponse is the response from GET /slurmdb/{version}/qos/
type QOSResponse struct {
	QOS      []QOS          `json:"qos"`
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// QOS represents a Slurm Quality of Service.
// The structure mirrors the Slurm API's nested JSON format.
type QOS struct {
	Name           string      `json:"name"`
	Description    string      `json:"description,omitempty"`
	Priority       *SlurmInt   `json:"priority,omitempty"`
	Flags          []string    `json:"flags,omitempty"`
	Limits         *QOSLimits  `json:"limits,omitempty"`
	Preempt        *QOSPreempt `json:"preempt,omitempty"`
	UsageFactor    *SlurmFloat `json:"usage_factor,omitempty"`
	UsageThreshold *SlurmFloat `json:"usage_threshold,omitempty"`
}

// QOSTresPer holds per-entity TRES limits on a QOS.
type QOSTresPer struct {
	Account []TRES `json:"account,omitempty"`
	Job     []TRES `json:"job,omitempty"`
	Node    []TRES `json:"node,omitempty"`
	User    []TRES `json:"user,omitempty"`
}

// QOSTresMinsPer holds per-entity TRES-minutes limits on a QOS.
type QOSTresMinsPer struct {
	QOS     []TRES `json:"qos,omitempty"`
	Job     []TRES `json:"job,omitempty"`
	Account []TRES `json:"account,omitempty"`
	User    []TRES `json:"user,omitempty"`
}

// QOSTresMins holds TRES-minutes limits on a QOS.
type QOSTresMins struct {
	Total []TRES          `json:"total,omitempty"`
	Per   *QOSTresMinsPer `json:"per,omitempty"`
}

// QOSTresLimits holds all TRES limits (total, per-entity, minutes) on a QOS.
type QOSTresLimits struct {
	Total   []TRES       `json:"total,omitempty"`
	Minutes *QOSTresMins `json:"minutes,omitempty"`
	Per     *QOSTresPer  `json:"per,omitempty"`
}

// QOSMinTresPer holds per-job minimum TRES limits.
type QOSMinTresPer struct {
	Job []TRES `json:"job,omitempty"`
}

// QOSMinTres holds minimum TRES limits.
type QOSMinTres struct {
	Per *QOSMinTresPer `json:"per,omitempty"`
}

// QOSLimitsMin holds minimum limits for a QOS.
type QOSLimitsMin struct {
	TRES *QOSMinTres `json:"tres,omitempty"`
}

// QOSJobsPer holds per-entity running-job count limits.
type QOSJobsPer struct {
	Account *SlurmInt `json:"account,omitempty"`
	User    *SlurmInt `json:"user,omitempty"`
}

// QOSJobsActiveJobsPer holds per-entity submit-job count limits.
type QOSJobsActiveJobsPer struct {
	Account *SlurmInt `json:"account,omitempty"`
	User    *SlurmInt `json:"user,omitempty"`
}

// QOSJobsActiveJobs holds submit-job limits within the jobs node.
type QOSJobsActiveJobs struct {
	Per *QOSJobsActiveJobsPer `json:"per,omitempty"`
}

// QOSJobs holds job count limits for a QOS.
type QOSJobs struct {
	Count      *SlurmInt          `json:"count,omitempty"`
	ActiveJobs *QOSJobsActiveJobs `json:"active_jobs,omitempty"`
	Per        *QOSJobsPer        `json:"per,omitempty"`
}

// QOSActiveJobs holds the QOS-wide submit-job count limit.
type QOSActiveJobs struct {
	Count *SlurmInt `json:"count,omitempty"`
}

// QOSLimits contains the limit configuration for a QOS.
type QOSLimits struct {
	GraceTime int           `json:"grace_time,omitempty"`
	Max       *QOSLimitsMax `json:"max,omitempty"`
	Min       *QOSLimitsMin `json:"min,omitempty"`
}

// QOSLimitsMax contains the max limits.
type QOSLimitsMax struct {
	WallClock  *QOSWallClock  `json:"wall_clock,omitempty"`
	TRES       *QOSTresLimits `json:"tres,omitempty"`
	Jobs       *QOSJobs       `json:"jobs,omitempty"`
	ActiveJobs *QOSActiveJobs `json:"active_jobs,omitempty"`
}

// QOSWallClock contains wall clock limits.
type QOSWallClock struct {
	Per *QOSWallClockPer `json:"per,omitempty"`
}

// QOSWallClockPer contains per-job and per-QOS wall clock limits.
type QOSWallClockPer struct {
	Job *SlurmInt `json:"job,omitempty"`
	QOS *SlurmInt `json:"qos,omitempty"`
}

// QOSPreempt contains preemption settings for a QOS.
type QOSPreempt struct {
	List       []string  `json:"list,omitempty"`
	Mode       []string  `json:"mode,omitempty"`
	ExemptTime *SlurmInt `json:"exempt_time,omitempty"`
}

// GetQOS returns a single QOS by name.
func (c *Client) GetQOS(ctx context.Context, name string) (*QOS, error) {
	path := c.slurmdbPath(fmt.Sprintf("qos/%s", url.PathEscape(name)))
	data, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp QOSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal QOS response: %w", err)
	}
	if len(resp.QOS) == 0 {
		return nil, nil // not found
	}
	return &resp.QOS[0], nil
}

// CreateQOS creates or updates a QOS entry.
func (c *Client) CreateQOS(ctx context.Context, qos QOS) error {
	body := map[string][]QOS{
		"qos": {qos},
	}
	_, err := c.doRequest(ctx, http.MethodPost, c.slurmdbPath("qos/"), body)
	return err
}

// DeleteQOS deletes a QOS by name.
func (c *Client) DeleteQOS(ctx context.Context, name string) error {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	path := c.slurmdbPath(fmt.Sprintf("qos/%s", url.PathEscape(name)))
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}
