package guard

import (
	"context"
)

// Guard is the interface that all guardrail implementations must satisfy.
// Each guard validates a text input and returns a result indicating pass/fail.
type Guard interface {
	// Type returns the guard type identifier (e.g., "blocklist", "regex").
	Type() string

	// Name returns the human-readable name of this guard instance.
	Name() string

	// Validate checks the input text against this guard's rules.
	// Returns a Result indicating whether the input passed or failed.
	Validate(ctx context.Context, input string) (*Result, error)
}

// Result holds the outcome of a single guard validation.
type Result struct {
	// GuardName is the human-readable name of the guard that produced this result.
	GuardName string `json:"guard_name"`

	// GuardType is the type identifier of the guard (e.g., "blocklist", "regex").
	GuardType string `json:"guard_type"`

	// Passed indicates whether the input passed this guard's validation.
	Passed bool `json:"passed"`

	// Message provides a human-readable explanation of the result.
	Message string `json:"message"`

	// Details contains guard-type-specific metadata about the validation.
	// For blocklist: {"matched_words": ["word1", "word2"]}
	// For regex: {"matched_patterns": ["\\bfoo\\b"]}
	Details map[string]interface{} `json:"details,omitempty"`

	// DurationMs is how long this guard took to execute, in milliseconds.
	DurationMs int64 `json:"duration_ms"`

	// Shadow is true when this result came from a shadow (monitor-only) guard:
	// it ran and its Passed value is real, but it did not affect the request's
	// accept/reject decision. Recorded so a would-be rejection is visible in
	// audit before the guard is switched to enforcing.
	Shadow bool `json:"shadow,omitempty"`
}

// unwrapper is implemented by guard decorators so that marker interfaces
// (FailOpenGuard, ShadowGuard) can be detected regardless of wrap order.
type unwrapper interface {
	Unwrap() Guard
}

// guardAs reports whether g — or any guard it wraps — satisfies T.
func guardAs[T any](g Guard) (T, bool) {
	for {
		if v, ok := g.(T); ok {
			return v, true
		}
		u, ok := g.(unwrapper)
		if !ok {
			var zero T
			return zero, false
		}
		g = u.Unwrap()
	}
}

// FailOpenGuard is implemented by guards that should be treated as passed
// when they error (e.g. an external moderation API outage), instead of the
// default fail-closed behavior.
type FailOpenGuard interface {
	Guard
	FailOpen() bool
}

// failOpen wraps a guard to mark it fail-open on error.
type failOpen struct {
	Guard
}

// FailOpen reports that errors from this guard should count as passed.
func (failOpen) FailOpen() bool { return true }

// Unwrap returns the wrapped guard.
func (f failOpen) Unwrap() Guard { return f.Guard }

// WithFailOpen wraps a guard so that execution errors count as passed
// (marked as errored in the result) rather than failing the request.
func WithFailOpen(g Guard) Guard {
	return failOpen{g}
}

// ShadowGuard is implemented by guards running in shadow (monitor-only) mode:
// they execute and their result is recorded, but they do not count toward the
// request's accept/reject decision.
type ShadowGuard interface {
	Guard
	Shadow() bool
}

// shadow wraps a guard to mark it monitor-only.
type shadow struct {
	Guard
}

// Shadow reports that this guard does not affect the accept/reject decision.
func (shadow) Shadow() bool { return true }

// Unwrap returns the wrapped guard.
func (s shadow) Unwrap() Guard { return s.Guard }

// WithShadow wraps a guard so it runs and is recorded but never blocks traffic.
func WithShadow(g Guard) Guard {
	return shadow{g}
}

// PassCriteria defines how multiple guard results are evaluated.
type PassCriteria string

const (
	// CriteriaAll requires every guard to pass.
	CriteriaAll PassCriteria = "all"

	// CriteriaAny requires at least one guard to pass.
	CriteriaAny PassCriteria = "any"

	// CriteriaNofM requires at least N guards to pass out of M total.
	CriteriaNofM PassCriteria = "n_of_m"
)

// EngineResult holds the aggregated outcome of running all guards for a phase.
type EngineResult struct {
	// Passed indicates whether the overall evaluation passed based on the criteria.
	Passed bool `json:"passed"`

	// Criteria is the pass criteria that was applied.
	Criteria PassCriteria `json:"criteria"`

	// Threshold is the N value for n_of_m criteria (nil for all/any).
	Threshold *int `json:"threshold,omitempty"`

	// Results contains the individual result from each guard.
	Results []*Result `json:"results"`

	// TotalDurationMs is the wall-clock time for the entire evaluation, in milliseconds.
	TotalDurationMs int64 `json:"total_duration_ms"`

	// PassCount is how many guards passed.
	PassCount int `json:"pass_count"`

	// FailCount is how many guards failed.
	FailCount int `json:"fail_count"`
}
