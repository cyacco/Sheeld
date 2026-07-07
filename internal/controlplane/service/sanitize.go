package service

import "strings"

// redactedValue replaces secret config fields in API responses. The stored
// config keeps the real value; only the response is scrubbed.
const redactedValue = "***"

// SanitizeConfig returns a copy of a guard/transformer config with secret
// fields redacted, so GET responses never leak provider API keys or auth
// headers. A field is treated as secret if its key is exactly "api_key" or
// "headers", or ends in "_key" (e.g. a future "signing_key"). The input map
// is not mutated.
func SanitizeConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		if isSecretKey(k) {
			out[k] = redactedValue
			continue
		}
		out[k] = v
	}
	return out
}

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	return k == "api_key" || k == "headers" || strings.HasSuffix(k, "_key")
}

// PreserveRedactedSecrets makes redaction round-trip-safe on update: when an
// incoming config field carries the redacted sentinel (the client loaded a
// sanitized GET response and sent it back unchanged), the stored value is
// kept instead of overwriting the real secret with "***". Non-sentinel
// values pass through, so a genuinely new secret still updates.
func PreserveRedactedSecrets(incoming, stored map[string]interface{}) map[string]interface{} {
	if incoming == nil {
		return incoming
	}
	out := make(map[string]interface{}, len(incoming))
	for k, v := range incoming {
		if v == redactedValue && stored != nil {
			if prev, ok := stored[k]; ok {
				out[k] = prev
				continue
			}
		}
		out[k] = v
	}
	return out
}
