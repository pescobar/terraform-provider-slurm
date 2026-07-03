package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

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
	WCKey   string `json:"wckey,omitempty"`
}

// UserAssociationRequest is the body for POST /slurmdb/{version}/users_association/
// In API v0.0.42 the endpoint changed: it now takes association_condition with
// user/account lists, not a users+associations payload.
type UserAssociationRequest struct {
	AssociationCondition UserAssociationCondition `json:"association_condition"`
	User                 UserShort                `json:"user"`
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
