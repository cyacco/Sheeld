package processor

import "github.com/cyacco/Sheeld/internal/shared/urlpolicy"

// Tests build guards/transformers from configs pointing at loopback
// httptest servers, so allow private targets for the whole package.
func init() { urlpolicy.SetAllowPrivate(true) }
