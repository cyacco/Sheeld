package guard

import "context"

// CallMeta carries request context (which phase is running, for which
// source) to guards that forward it to external services.
type CallMeta struct {
	// Phase is "input" or "output".
	Phase string

	// SourceRoute is the route of the source being proxied.
	SourceRoute string
}

type callMetaKey struct{}

// WithCallMeta attaches call metadata to the context.
func WithCallMeta(ctx context.Context, m CallMeta) context.Context {
	return context.WithValue(ctx, callMetaKey{}, m)
}

// CallMetaFrom extracts call metadata from the context.
func CallMetaFrom(ctx context.Context) (CallMeta, bool) {
	m, ok := ctx.Value(callMetaKey{}).(CallMeta)
	return m, ok
}
