package service

import "testing"

func boolPtr(b bool) *bool { return &b }

func int32Ptr(v int32) *int32 { return &v }

func TestValidateCriteria(t *testing.T) {
	tests := []struct {
		name      string
		criteria  string
		threshold *int32
		want      string
		wantErr   bool
	}{
		{"omitted defaults to all", "", nil, "all", false},
		{"any preserved", "any", nil, "any", false},
		{"n_of_m with threshold", "n_of_m", int32Ptr(2), "n_of_m", false},
		{"n_of_m without threshold", "n_of_m", nil, "", true},
		{"n_of_m with zero threshold", "n_of_m", int32Ptr(0), "", true},
		{"unknown criteria", "bogus", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateCriteria("input", tt.criteria, tt.threshold)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("validateCriteria(%q, %v) = (%q, %v), want (%q, err=%v)",
					tt.criteria, tt.threshold, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestGuardrailDefaults(t *testing.T) {
	tests := []struct {
		name        string
		phase       string
		enabled     *bool
		wantPhase   string
		wantEnabled bool
	}{
		{"all omitted", "", nil, "input", true},
		{"explicit false enabled", "output", boolPtr(false), "output", false},
		{"phase preserved", "both", nil, "both", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase, enabled := guardrailDefaults(tt.phase, tt.enabled)
			if phase != tt.wantPhase || enabled != tt.wantEnabled {
				t.Errorf("guardrailDefaults(%q, %v) = (%q, %v), want (%q, %v)",
					tt.phase, tt.enabled, phase, enabled, tt.wantPhase, tt.wantEnabled)
			}
		})
	}
}
