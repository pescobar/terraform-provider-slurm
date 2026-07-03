package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// AccountResponse is the response from GET /slurmdb/{version}/accounts/
type AccountResponse struct {
	Accounts []Account      `json:"accounts"`
	Errors   []SlurmError   `json:"errors"`
	Warnings []SlurmWarning `json:"warnings"`
}

// Account represents a Slurm account.
type Account struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Organization  string   `json:"organization"`
	ParentAccount string   `json:"parent_account,omitempty"`
	Coordinators  []string `json:"coordinators,omitempty"`
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
	Account              AccountShort                `json:"account"`
}

// AccountAssociationCondition specifies which account+cluster combinations to create.
//
// The endpoint also accepts an "association" field for inline limits, but this
// provider deliberately does not use it: posting limits via
// /accounts_association/ races with concurrent user-association updates and
// can drop QOS entries from the account-level association. Limits are written
// in a follow-up call to /associations/ instead — see resources/account.go.
type AccountAssociationCondition struct {
	Accounts []string `json:"accounts"`
	Clusters []string `json:"clusters,omitempty"`
}

// AccountShort is the minimal account object accepted by accounts_association.
type AccountShort struct {
	Description  string `json:"description,omitempty"`
	Organization string `json:"organization,omitempty"`
	Parent       string `json:"parent,omitempty"`
}

// CreateAccountWithAssociation creates an account and its cluster-level
// association atomically via POST /accounts_association/.
func (c *Client) CreateAccountWithAssociation(req AccountAssociationRequest) error {
	_, err := c.doRequest(http.MethodPost, c.slurmdbPath("accounts_association/"), req)
	return err
}
