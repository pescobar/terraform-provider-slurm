package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ClusterResponse is the response from GET /slurmdb/{version}/clusters/
type ClusterResponse struct {
	Clusters []Cluster      `json:"clusters"`
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// Cluster represents a Slurm cluster.
type Cluster struct {
	Name  string        `json:"name"`
	Nodes string        `json:"nodes,omitempty"`
	Flags []string      `json:"flags,omitempty"`
	Tres  []interface{} `json:"tres,omitempty"`
}

// GetCluster returns a single cluster by name.
func (c *Client) GetCluster(name string) (*Cluster, error) {
	path := c.slurmdbPath(fmt.Sprintf("cluster/%s", url.PathEscape(name)))
	data, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp ClusterResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster response: %w", err)
	}
	if len(resp.Clusters) == 0 {
		return nil, nil // not found
	}
	return &resp.Clusters[0], nil
}

// CreateCluster creates or updates a cluster.
func (c *Client) CreateCluster(cluster Cluster) error {
	body := map[string][]Cluster{
		"clusters": {cluster},
	}
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("clusters/"), body)
	return err
}

// EnsureCluster registers the cluster in slurmdbd only if it does not already
// exist. We intentionally do NOT update an existing cluster to avoid overwriting
// its TRES configuration (set by slurmctld at startup) with an empty object.
func (c *Client) EnsureCluster() error {
	existing, err := c.GetCluster(c.Cluster)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	return c.CreateCluster(Cluster{Name: c.Cluster})
}
