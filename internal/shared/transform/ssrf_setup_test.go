package transform

import "github.com/cyacco/Sheeld/internal/shared/urlpolicy"

// Tests point transformers at httptest servers (loopback) and non-resolving
// hostnames, so allow private targets for the whole package.
func init() { urlpolicy.SetAllowPrivate(true) }
