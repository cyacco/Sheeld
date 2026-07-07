package urlpolicy

import "testing"

func TestValidatePublicHTTPURL_Public(t *testing.T) {
	SetAllowPrivate(false)
	defer SetAllowPrivate(false)

	// Public IP literals and a public hostname should pass.
	for _, raw := range []string{
		"https://api.openai.com/v1",
		"http://8.8.8.8:3000",
		"https://1.1.1.1",
	} {
		if err := ValidatePublicHTTPURL(raw, "url"); err != nil {
			t.Errorf("%q should be allowed, got %v", raw, err)
		}
	}
}

func TestValidatePublicHTTPURL_RejectsPrivate(t *testing.T) {
	SetAllowPrivate(false)
	defer SetAllowPrivate(false)

	tests := []string{
		"http://127.0.0.1:3000",    // loopback
		"http://localhost:8000",    // loopback via name
		"http://10.0.0.5",          // RFC 1918
		"http://192.168.1.10:3000", // RFC 1918
		"http://172.16.0.1",        // RFC 1918
		"http://169.254.169.254",   // link-local (cloud metadata)
		"http://100.64.0.1",        // CGNAT
		"http://0.0.0.0",           // unspecified
		"https://[::1]:3000",       // IPv6 loopback
	}
	for _, raw := range tests {
		if err := ValidatePublicHTTPURL(raw, "url"); err == nil {
			t.Errorf("%q should be rejected as private/loopback", raw)
		}
	}
}

func TestValidatePublicHTTPURL_Malformed(t *testing.T) {
	SetAllowPrivate(false)
	for _, raw := range []string{"", "ftp://example.com", "not a url", "https://"} {
		if err := ValidatePublicHTTPURL(raw, "url"); err == nil {
			t.Errorf("%q should be rejected as malformed", raw)
		}
	}
}

func TestValidatePublicHTTPURL_AllowPrivate(t *testing.T) {
	SetAllowPrivate(true)
	defer SetAllowPrivate(false)

	// With the opt-in, internal targets are permitted, but the URL must
	// still be well-formed http(s).
	if err := ValidatePublicHTTPURL("http://presidio-analyzer:3000", "url"); err != nil {
		t.Errorf("private host should be allowed when opted in, got %v", err)
	}
	if err := ValidatePublicHTTPURL("ftp://x", "url"); err == nil {
		t.Error("non-http scheme should still be rejected when private allowed")
	}
}
