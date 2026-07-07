package transform

import "github.com/sheeld/sheeld/internal/shared/urlpolicy"

// Tests point transformers at httptest servers (loopback) and non-resolving
// hostnames, so allow private targets for the whole package.
func init() { urlpolicy.SetAllowPrivate(true) }
