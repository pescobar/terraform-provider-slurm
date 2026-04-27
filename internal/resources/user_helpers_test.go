package resources

import (
	"errors"
	"strings"
	"testing"
)

func TestIsQOSAccessError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rule1 phrase: would not have access",
			err:      errors.New("user u_qos would not have access to their default qos with account dept_a"),
			expected: true,
		},
		{
			name:     "rule2 phrase: don't have access",
			err:      errors.New("slurm API error (HTTP 200): user u_fs_dqos don't have access to their default qos with account team_a1"),
			expected: true,
		},
		{
			name:     "unrelated slurm error",
			err:      errors.New("slurm API error (HTTP 500): slurmdb_users_add failed"),
			expected: false,
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "empty message",
			err:      errors.New(""),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isQOSAccessError(tc.err)
			if got != tc.expected {
				t.Errorf("isQOSAccessError(%v) = %v, want %v", tc.err, got, tc.expected)
			}
		})
	}
}

func TestAssocErrorDetail_QOSAccessErrorIncludesHint(t *testing.T) {
	err := errors.New("user u_qos would not have access to their default qos with account dept_a")
	detail := assocErrorDetail("User 'u_qos' was created but associations failed", err)

	if !strings.Contains(detail, "User 'u_qos' was created but associations failed") {
		t.Error("expected prefix in output")
	}
	if !strings.Contains(detail, err.Error()) {
		t.Error("expected original error message in output")
	}
	if !strings.Contains(detail, "Slurm enforces two QOS access rules") {
		t.Error("expected hint header in output")
	}
	if !strings.Contains(detail, "Rule 1") {
		t.Error("expected Rule 1 description in hint")
	}
	if !strings.Contains(detail, "Rule 2") {
		t.Error("expected Rule 2 description in hint")
	}
}

func TestAssocErrorDetail_NonQOSErrorHasNoHint(t *testing.T) {
	err := errors.New("slurm API error (HTTP 500): slurmdb_users_add failed")
	detail := assocErrorDetail("Failed to create associations for user 'bob'", err)

	if !strings.Contains(detail, "Failed to create associations for user 'bob'") {
		t.Error("expected prefix in output")
	}
	if !strings.Contains(detail, err.Error()) {
		t.Error("expected original error message in output")
	}
	if strings.Contains(detail, "Slurm enforces two QOS access rules") {
		t.Error("unexpected QOS hint for a non-QOS error")
	}
}

func TestAssocErrorDetail_BothQOSPhrases(t *testing.T) {
	// Verify that both Slurm error phrasings trigger the hint, since different
	// Slurm versions may emit slightly different messages.
	phrases := []string{
		"would not have access to their default qos",
		"don't have access to their default qos",
	}
	for _, phrase := range phrases {
		t.Run(phrase, func(t *testing.T) {
			err := errors.New("user u_test " + phrase + " with account acct")
			detail := assocErrorDetail("op failed", err)
			if !strings.Contains(detail, "Slurm enforces two QOS access rules") {
				t.Errorf("expected hint for phrase %q", phrase)
			}
		})
	}
}
