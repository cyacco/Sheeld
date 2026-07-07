package service

import "testing"

func TestSanitizeConfig(t *testing.T) {
	in := map[string]interface{}{
		"api_key":         "sk-secret",
		"base_url":        "https://api.openai.com/v1",
		"headers":         map[string]interface{}{"Authorization": "Bearer x"},
		"signing_key":     "shh",
		"model":           "gpt-4o-mini",
		"score_threshold": 0.5,
	}
	out := SanitizeConfig(in)

	for _, k := range []string{"api_key", "headers", "signing_key"} {
		if out[k] != redactedValue {
			t.Errorf("%q not redacted: got %v", k, out[k])
		}
	}
	if out["base_url"] != "https://api.openai.com/v1" || out["model"] != "gpt-4o-mini" || out["score_threshold"] != 0.5 {
		t.Errorf("non-secret fields altered: %v", out)
	}
	// Input not mutated.
	if in["api_key"] != "sk-secret" {
		t.Error("input map was mutated")
	}
	if SanitizeConfig(nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestPreserveRedactedSecrets(t *testing.T) {
	stored := map[string]interface{}{"api_key": "sk-real", "base_url": "https://old"}

	// Sentinel is replaced with the stored value; a genuinely new secret
	// passes through; non-secret changes pass through.
	incoming := map[string]interface{}{
		"api_key":  redactedValue,
		"base_url": "https://new",
	}
	out := PreserveRedactedSecrets(incoming, stored)
	if out["api_key"] != "sk-real" {
		t.Errorf("redacted api_key should keep stored value, got %v", out["api_key"])
	}
	if out["base_url"] != "https://new" {
		t.Errorf("changed field should pass through, got %v", out["base_url"])
	}

	incoming2 := map[string]interface{}{"api_key": "sk-new"}
	if PreserveRedactedSecrets(incoming2, stored)["api_key"] != "sk-new" {
		t.Error("a new non-sentinel secret should update")
	}

	// Sentinel with no stored counterpart passes through unchanged.
	out3 := PreserveRedactedSecrets(map[string]interface{}{"api_key": redactedValue}, nil)
	if out3["api_key"] != redactedValue {
		t.Errorf("no stored config: sentinel passes through, got %v", out3["api_key"])
	}
}
