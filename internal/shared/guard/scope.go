package guard

import "context"

// scopeAllMessages wraps a guard so it validates the whole serialized
// conversation instead of the last user message. It only takes effect on
// the input phase (output guards already see the full response text); when
// no all-messages text is present in the context, it delegates unchanged.
type scopeAllMessages struct {
	Guard
}

// WithScopeAllMessages wraps a guard with scope: all_messages semantics.
// Wrap order matters: apply this before WithFailOpen so the engine's
// FailOpenGuard type assertion sees the outermost wrapper.
func WithScopeAllMessages(g Guard) Guard {
	return scopeAllMessages{g}
}

func (s scopeAllMessages) Validate(ctx context.Context, input string) (*Result, error) {
	if meta, ok := CallMetaFrom(ctx); ok && meta.Phase == "input" && meta.AllMessagesText != "" {
		input = meta.AllMessagesText
	}
	return s.Guard.Validate(ctx, input)
}
