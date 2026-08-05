package domain

import (
	"errors"
	"testing"
)

func TestAlertConstructors(t *testing.T) {
	target := Target{Name: "api", URL: "https://api.example.com/health", ExpectedStatus: 200}

	failed := CheckResult{Target: target, ActualStatus: 500, Error: errors.New("status mismatch")}
	recovered := CheckResult{Target: target, Success: true, ActualStatus: 200}

	tests := []struct {
		name         string
		alert        Alert
		wantType     AlertType
		wantSeverity Severity
		wantMessage  string
	}{
		{
			name:         "failure carries the check error",
			alert:        NewFailureAlert(failed),
			wantType:     AlertTypeFailure,
			wantSeverity: SeverityCritical,
			wantMessage:  "status mismatch",
		},
		{
			name:         "failure without an error falls back to a default message",
			alert:        NewFailureAlert(CheckResult{Target: target}),
			wantType:     AlertTypeFailure,
			wantSeverity: SeverityCritical,
			wantMessage:  "Health check failed",
		},
		{
			name:         "reminder",
			alert:        NewReminderAlert(failed),
			wantType:     AlertTypeReminder,
			wantSeverity: SeverityWarning,
			wantMessage:  "status mismatch",
		},
		{
			name:         "recovery",
			alert:        NewRecoveryAlert(recovered),
			wantType:     AlertTypeRecovery,
			wantSeverity: SeverityInfo,
			wantMessage:  "Health check recovered",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.alert.Type != tc.wantType {
				t.Errorf("Type = %v, want %v", tc.alert.Type, tc.wantType)
			}
			if tc.alert.Severity != tc.wantSeverity {
				t.Errorf("Severity = %v, want %v", tc.alert.Severity, tc.wantSeverity)
			}
			if tc.alert.Message != tc.wantMessage {
				t.Errorf("Message = %q, want %q", tc.alert.Message, tc.wantMessage)
			}
			if tc.alert.Target.Name != target.Name {
				t.Errorf("Target.Name = %q, want %q", tc.alert.Target.Name, target.Name)
			}
			if tc.alert.Timestamp.IsZero() {
				t.Error("Timestamp is unset")
			}
		})
	}
}

// The zero value must read as "not checked yet" rather than "healthy".
func TestHealthStatusZeroValue(t *testing.T) {
	var status HealthStatus

	if status != StatusUnknown {
		t.Errorf("zero value = %v, want StatusUnknown", status)
	}
	if status.String() != "unknown" {
		t.Errorf("String() = %q, want %q", status.String(), "unknown")
	}
}

func TestStringers(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{AlertTypeFailure.String(), "FAILURE"},
		{AlertTypeRecovery.String(), "RECOVERY"},
		{AlertTypeReminder.String(), "REMINDER"},
		{AlertType(99).String(), "UNKNOWN"},
		{SeverityInfo.String(), "INFO"},
		{SeverityWarning.String(), "WARNING"},
		{SeverityCritical.String(), "CRITICAL"},
		{Severity(99).String(), "UNKNOWN"},
		{StatusHealthy.String(), "healthy"},
		{StatusUnhealthy.String(), "unhealthy"},
		{HealthStatus(99).String(), "unknown"},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
