// Package client provides an HTTP client for the Slurm REST API (slurmrestd).
//
// It handles JWT authentication, API versioning, and provides typed methods
// for each slurmdb endpoint used by the provider.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Client is an HTTP client for the Slurm REST API.
type Client struct {
	// BaseURL is the slurmrestd endpoint (e.g. "http://localhost:6820")
	BaseURL string

	// Token is the JWT token for authentication
	Token string

	// Cluster is the Slurm cluster name
	Cluster string

	// APIVersion is the data_parser version (e.g. "v0.0.42")
	APIVersion string

	// HTTPClient is the underlying HTTP client
	HTTPClient *http.Client

	// deleteMu serializes all delete operations. slurmdbd uses optimistic locking
	// and returns MySQL error 1020 when concurrent deletes race on cross-row
	// updates (e.g. QOS preempt references, account association cascades).
	deleteMu sync.Mutex
}

// NewClient creates a new Slurm REST API client.
func NewClient(baseURL, token, cluster, apiVersion string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		Cluster:    cluster,
		APIVersion: apiVersion,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// APIError represents an error returned by the Slurm REST API.
type APIError struct {
	StatusCode int
	Body       string
	Errors     []SlurmError
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		msg := fmt.Sprintf("slurm API error (HTTP %d):", e.StatusCode)
		for _, slurmErr := range e.Errors {
			msg += fmt.Sprintf(" %s", slurmErr.Description)
			// Include the Error field when it adds detail beyond Description.
			// Slurm sometimes puts the specific constraint message there
			// (e.g. QOS access violations) while Description is a generic
			// "slurmdb_X failed" string.
			if slurmErr.Error != "" && slurmErr.Error != slurmErr.Description {
				msg += fmt.Sprintf(" (%s)", slurmErr.Error)
			}
		}
		return msg
	}
	return fmt.Sprintf("slurm API error (HTTP %d): %s", e.StatusCode, e.Body)
}

// SlurmError is an individual error from a Slurm API response.
type SlurmError struct {
	Description string `json:"description"`
	Source      string `json:"source"`
	Error       string `json:"error"`
	ErrorNumber int    `json:"error_number"`
}

// SlurmWarning is an individual warning from a Slurm API response.
type SlurmWarning struct {
	Description string `json:"description"`
	Source      string `json:"source"`
}

// baseResponse contains the common fields in every Slurm API response.
type baseResponse struct {
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// slurmdbPath builds a URL path for a slurmdb endpoint.
// Example: slurmdbPath("accounts") => "/slurmdb/v0.0.42/accounts/"
func (c *Client) slurmdbPath(endpoint string) string {
	return fmt.Sprintf("/slurmdb/%s/%s", c.APIVersion, endpoint)
}

// doRequest performs an HTTP request against slurmrestd.
// It sets the JWT auth header and content type, then checks the response
// for Slurm-level errors.
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	var jsonBody []byte
	if body != nil {
		var err error
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	var bodyReader io.Reader
	if jsonBody != nil {
		bodyReader = bytes.NewReader(jsonBody)
	}

	reqURL := fmt.Sprintf("%s%s", c.BaseURL, path)
	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-SLURM-USER-TOKEN", c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP-level errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
		var base baseResponse
		if json.Unmarshal(respBody, &base) == nil {
			apiErr.Errors = base.Errors
		}
		return nil, apiErr
	}

	// Even on 200, Slurm may return errors in the body
	var base baseResponse
	if err := json.Unmarshal(respBody, &base); err == nil && len(base.Errors) > 0 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Errors:     base.Errors,
		}
	}

	return respBody, nil
}

// ---------------------------------------------------------------------
// Cluster endpoints
// ---------------------------------------------------------------------

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

// GetClusters returns all clusters.
func (c *Client) GetClusters() (*ClusterResponse, error) {
	data, err := c.doRequest(http.MethodGet, c.slurmdbPath("clusters/"), nil)
	if err != nil {
		return nil, err
	}
	var resp ClusterResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal clusters response: %w", err)
	}
	return &resp, nil
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

// DeleteCluster deletes a cluster by name.
func (c *Client) DeleteCluster(name string) error {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	path := c.slurmdbPath(fmt.Sprintf("cluster/%s", url.PathEscape(name)))
	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

// ---------------------------------------------------------------------
// Account endpoints
// ---------------------------------------------------------------------

// AccountResponse is the response from GET /slurmdb/{version}/accounts/
type AccountResponse struct {
	Accounts []Account      `json:"accounts"`
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// Account represents a Slurm account.
type Account struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Organization  string   `json:"organization,omitempty"`
	ParentAccount string   `json:"parent_account,omitempty"`
	Coordinators  []string `json:"coordinators,omitempty"`
}

// GetAccounts returns all accounts.
func (c *Client) GetAccounts() (*AccountResponse, error) {
	data, err := c.doRequest(http.MethodGet, c.slurmdbPath("accounts/"), nil)
	if err != nil {
		return nil, err
	}
	var resp AccountResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal accounts response: %w", err)
	}
	return &resp, nil
}

// GetAccount returns a single account by name.
func (c *Client) GetAccount(name string) (*Account, error) {
	path := c.slurmdbPath(fmt.Sprintf("account/%s", url.PathEscape(name)))
	data, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp AccountResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account response: %w", err)
	}
	if len(resp.Accounts) == 0 {
		return nil, nil // not found
	}
	return &resp.Accounts[0], nil
}

// CreateAccount creates or updates an account.
func (c *Client) CreateAccount(account Account) error {
	body := map[string][]Account{
		"accounts": {account},
	}
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("accounts/"), body)
	return err
}

// DeleteAccount deletes an account by name.
func (c *Client) DeleteAccount(name string) error {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	path := c.slurmdbPath(fmt.Sprintf("account/%s", url.PathEscape(name)))
	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

// AccountAssociationRequest is the body for POST /slurmdb/{version}/accounts_association/
// This endpoint atomically creates an account and its cluster-level association.
type AccountAssociationRequest struct {
	AssociationCondition AccountAssociationCondition `json:"association_condition"`
	Account              AccountShort               `json:"account"`
}

// AccountAssociationCondition specifies which account+cluster combinations to create.
type AccountAssociationCondition struct {
	Accounts    []string     `json:"accounts"`
	Clusters    []string     `json:"clusters,omitempty"`
	Association *AssocRecSet `json:"association,omitempty"`
}

// AccountShort is the minimal account object accepted by accounts_association.
type AccountShort struct {
	Description  string `json:"description,omitempty"`
	Organization string `json:"organization,omitempty"`
	Parent       string `json:"parent,omitempty"`
}

// AssocRecSet holds the writable association limit fields accepted by
// accounts_association and users_association. Field names differ from the
// Association struct because the _association endpoints use a separate schema.
type AssocRecSet struct {
	Fairshare  *int      `json:"fairshare,omitempty"`
	DefaultQOS string    `json:"defaultqos,omitempty"`
	QOSLevel   []string  `json:"qoslevel,omitempty"`
	MaxJobs    *SlurmInt `json:"maxjobs,omitempty"`
}

// CreateAccountWithAssociation creates an account and its cluster-level
// association atomically via POST /accounts_association/.
func (c *Client) CreateAccountWithAssociation(req AccountAssociationRequest) error {
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("accounts_association/"), req)
	return err
}

// ---------------------------------------------------------------------
// QOS endpoints
// ---------------------------------------------------------------------

// QOSResponse is the response from GET /slurmdb/{version}/qos/
type QOSResponse struct {
	QOS      []QOS          `json:"qos"`
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// QOS represents a Slurm Quality of Service.
// The structure mirrors the Slurm API's nested JSON format.
type QOS struct {
	Name             string      `json:"name"`
	Description      string      `json:"description,omitempty"`
	Priority         *SlurmInt   `json:"priority,omitempty"`
	Flags            []string    `json:"flags,omitempty"`
	Limits           *QOSLimits  `json:"limits,omitempty"`
	Preempt          *QOSPreempt `json:"preempt,omitempty"`
	UsageFactor      *SlurmFloat `json:"usage_factor,omitempty"`
	UsageThreshold   *SlurmFloat `json:"usage_threshold,omitempty"`
}

// SlurmInt represents Slurm's integer type which includes set/infinite flags.
// Omitting infinite when false matches slurmrestd's default and avoids sending
// an explicit "infinite":false that can confuse slurmdbd's query-back logic.
type SlurmInt struct {
	Number   int  `json:"number"`
	Set      bool `json:"set"`
	Infinite bool `json:"infinite,omitempty"`
}

// SlurmFloat is like SlurmInt but for API fields that Slurm serialises as a
// JSON float (e.g. usage_factor returns 1.0, not 1). Using float64 avoids
// "cannot unmarshal number 1.0 into Go struct field … of type int".
type SlurmFloat struct {
	Number   float64 `json:"number"`
	Set      bool    `json:"set"`
	Infinite bool    `json:"infinite,omitempty"`
}

// TRES represents a Trackable Resource (cpu, mem, gres, …) with a count limit.
type TRES struct {
	Type  string `json:"type"`
	Name  string `json:"name,omitempty"`
	ID    int    `json:"id,omitempty"`
	Count int64  `json:"count"`
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

// GetAllQOS returns all QOS entries.
func (c *Client) GetAllQOS() (*QOSResponse, error) {
	data, err := c.doRequest(http.MethodGet, c.slurmdbPath("qos/"), nil)
	if err != nil {
		return nil, err
	}
	var resp QOSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal QOS response: %w", err)
	}
	return &resp, nil
}

// GetQOS returns a single QOS by name.
func (c *Client) GetQOS(name string) (*QOS, error) {
	path := c.slurmdbPath(fmt.Sprintf("qos/%s", url.PathEscape(name)))
	data, err := c.doRequest(http.MethodGet, path, nil)
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
func (c *Client) CreateQOS(qos QOS) error {
	body := map[string][]QOS{
		"qos": {qos},
	}
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("qos/"), body)
	return err
}

// DeleteQOS deletes a QOS by name.
func (c *Client) DeleteQOS(name string) error {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	path := c.slurmdbPath(fmt.Sprintf("qos/%s", url.PathEscape(name)))
	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

// ---------------------------------------------------------------------
// User endpoints
// ---------------------------------------------------------------------

// UserResponse is the response from GET /slurmdb/{version}/users/
type UserResponse struct {
	Users    []User         `json:"users"`
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// User represents a Slurm user.
type User struct {
	Name         string        `json:"name"`
	AdminLevel   []string      `json:"administrator_level,omitempty"`
	Default      *UserDefault  `json:"default,omitempty"`
	Associations []Association `json:"associations,omitempty"`
}

// UserDefault contains the user's default settings.
type UserDefault struct {
	Account string `json:"account,omitempty"`
}

// UserAssociationRequest is the body for POST /slurmdb/{version}/users_association/
// In API v0.0.42 the endpoint changed: it now takes association_condition with
// user/account lists, not a users+associations payload.
type UserAssociationRequest struct {
	AssociationCondition UserAssociationCondition `json:"association_condition"`
	User                 UserShort               `json:"user"`
}

// UserAssociationCondition specifies which user+account combinations to create.
type UserAssociationCondition struct {
	Users    []string `json:"users"`
	Accounts []string `json:"accounts"`
}

// UserShort is the minimal user object accepted by the users_association endpoint.
type UserShort struct {
	AdminLevel []string `json:"administrator_level,omitempty"`
}

// GetUsers returns all users.
func (c *Client) GetUsers() (*UserResponse, error) {
	data, err := c.doRequest(http.MethodGet, c.slurmdbPath("users/"), nil)
	if err != nil {
		return nil, err
	}
	var resp UserResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal users response: %w", err)
	}
	return &resp, nil
}

// GetUser returns a single user by name.
func (c *Client) GetUser(name string) (*User, error) {
	path := c.slurmdbPath(fmt.Sprintf("user/%s", url.PathEscape(name)))
	data, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp UserResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user response: %w", err)
	}
	if len(resp.Users) == 0 {
		return nil, nil // not found
	}
	return &resp.Users[0], nil
}

// CreateUserWithAssociation creates a user and its initial association in one call.
func (c *Client) CreateUserWithAssociation(req UserAssociationRequest) error {
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("users_association/"), req)
	return err
}

// UpdateUser updates user properties (not associations).
func (c *Client) UpdateUser(user User) error {
	body := map[string][]User{
		"users": {user},
	}
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("users/"), body)
	return err
}

// DeleteUser deletes a user by name.
func (c *Client) DeleteUser(name string) error {
	c.deleteMu.Lock()
	defer c.deleteMu.Unlock()
	path := c.slurmdbPath(fmt.Sprintf("user/%s", url.PathEscape(name)))
	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

// ---------------------------------------------------------------------
// Association endpoints
// ---------------------------------------------------------------------

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
	Default   *AssociationDefaults `json:"default,omitempty"`
	SharesRaw *int                 `json:"shares_raw,omitempty"`
	QOS       []string             `json:"qos,omitempty"`
	Max       *AssociationMax      `json:"max,omitempty"`
}

// AssociationDefaults contains default settings for an association.
type AssociationDefaults struct {
	QOS string `json:"qos,omitempty"`
}

// AssociationMax contains max limits for an association.
type AssociationMax struct {
	Jobs *AssociationMaxJobs `json:"jobs,omitempty"`
}

// AssociationMaxJobs contains the max jobs limits.
type AssociationMaxJobs struct {
	Per *AssociationMaxJobsPer `json:"per,omitempty"`
}

// AssociationMaxJobsPer contains the per-entity max jobs.
type AssociationMaxJobsPer struct {
	Count *SlurmInt `json:"count,omitempty"`
}

// GetAssociations returns all associations, optionally filtered by query params.
func (c *Client) GetAssociations(params map[string]string) (*AssociationResponse, error) {
	path := c.slurmdbPath("associations/")
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		path = fmt.Sprintf("%s?%s", path, q.Encode())
	}
	data, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp AssociationResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal associations response: %w", err)
	}
	return &resp, nil
}

// GetAssociation returns a single association by its key fields.
func (c *Client) GetAssociation(account, user, cluster, partition string) (*Association, error) {
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

	data, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp AssociationResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal association response: %w", err)
	}
	if len(resp.Associations) == 0 {
		return nil, nil // not found
	}
	return &resp.Associations[0], nil
}

// CreateAssociations creates or updates associations.
func (c *Client) CreateAssociations(associations []Association) error {
	body := map[string][]Association{
		"associations": associations,
	}
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("associations/"), body)
	return err
}

// DeleteAssociation deletes a single association by its key fields.
func (c *Client) DeleteAssociation(account, user, cluster, partition string) error {
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

	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

// Ping checks connectivity to slurmrestd.
func (c *Client) Ping() error {
	_, err := c.doRequest(http.MethodGet, c.slurmdbPath("diag/"), nil)
	return err
}
