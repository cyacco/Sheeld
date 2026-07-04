package service

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestSourceDefaults(t *testing.T) {
	tests := []struct {
		name         string
		passCriteria string
		enabled      *bool
		wantCriteria string
		wantEnabled  bool
	}{
		{"all omitted", "", nil, "all", true},
		{"explicit false enabled", "", boolPtr(false), "all", false},
		{"explicit true enabled", "any", boolPtr(true), "any", true},
		{"criteria preserved", "n_of_m", nil, "n_of_m", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			criteria, enabled := sourceDefaults(tt.passCriteria, tt.enabled)
			if criteria != tt.wantCriteria || enabled != tt.wantEnabled {
				t.Errorf("sourceDefaults(%q, %v) = (%q, %v), want (%q, %v)",
					tt.passCriteria, tt.enabled, criteria, enabled, tt.wantCriteria, tt.wantEnabled)
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
