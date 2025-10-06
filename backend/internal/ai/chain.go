package ai

import (
	"context"
	"strings"
)

type explainerChain struct {
	primary  Explainer
	fallback Explainer
}

// WithFallback returns an explainer that first tries the primary implementation and
// falls back to the provided explainer when the primary is unavailable or produces
// an unusable response.
func WithFallback(primary, fallback Explainer) Explainer {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return &explainerChain{primary: primary, fallback: fallback}
}

func (c *explainerChain) Enabled() bool {
	if c == nil {
		return false
	}
	if c.primary != nil && c.primary.Enabled() {
		return true
	}
	if c.fallback != nil && c.fallback.Enabled() {
		return true
	}
	return false
}

func (c *explainerChain) Explain(ctx context.Context, input ExplanationInput) (Decision, error) {
	if c == nil {
		return Decision{}, ErrDisabled
	}
	if c.primary != nil && c.primary.Enabled() {
		if decision, err := c.primary.Explain(ctx, input); err == nil {
			if strings.TrimSpace(decision.Narrative) != "" && decision.Recommendation != "" {
				return decision, nil
			}
		}
	}
	if c.fallback != nil && c.fallback.Enabled() {
		return c.fallback.Explain(ctx, input)
	}
	return Decision{}, ErrDisabled
}
