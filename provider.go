package featureflags

import "context"

// Provider wraps Client to implement the OpenFeature Go SDK provider interface.
// It applies the same evaluation logic as the backend.
type Provider struct {
	client *Client
}

// NewProvider creates a Provider backed by the given Client.
// The client should already be connected (or Connect called before evaluating).
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// IsEnabled checks whether a flag is enabled for the given tenant without
// the OpenFeature abstraction. Returns defaultValue if the flag is not found.
// tenantID is optional — omit or pass "" for flags that don't use partial rollout.
// For partially_enabled flags, an empty tenantID always returns false.
func (p *Provider) IsEnabled(flagKey string, defaultValue bool, tenantID ...string) bool {
	tid := ""
	if len(tenantID) > 0 {
		tid = tenantID[0]
	}
	val, ok := p.client.GetFlagValue(flagKey)
	if !ok {
		return defaultValue
	}
	return evaluate(val, EvaluationContext{TenantID: tid}).Value
}

// OnChange registers a callback for flagKey that fires immediately with the
// current evaluated value, then again on every SSE update. Returns a
// Subscription — call Unsubscribe when done to stop receiving callbacks.
// tenantID is optional — omit or pass "" for flags that don't use partial rollout.
func (p *Provider) OnChange(flagKey string, cb func(bool), defaultValue bool, tenantID ...string) *Subscription {
	tid := ""
	if len(tenantID) > 0 {
		tid = tenantID[0]
	}
	evalCtx := EvaluationContext{TenantID: tid}
	// Fire immediately with the current value.
	val, ok := p.client.GetFlagValue(flagKey)
	if ok {
		cb(evaluate(val, evalCtx).Value)
	} else {
		cb(defaultValue)
	}
	// Register for future updates.
	unsubscribe := p.client.subscribe(flagKey, func(v FlagValue) {
		cb(evaluate(v, evalCtx).Value)
	})
	return &Subscription{unsubscribe: unsubscribe}
}

// ResolveBooleanValue evaluates a boolean flag for the given context.
func (p *Provider) ResolveBooleanValue(ctx context.Context, flagKey string, defaultValue bool, evalCtx EvaluationContext) BoolResolutionDetail {
	val, ok := p.client.GetFlagValue(flagKey)
	if !ok {
		return BoolResolutionDetail{Value: defaultValue, Reason: ReasonFlagNotFound}
	}
	return evaluate(val, evalCtx)
}

// evaluate applies the flag evaluation logic — identical to the Go backend.
func evaluate(val FlagValue, evalCtx EvaluationContext) BoolResolutionDetail {
	switch val.State {
	case FlagStateEnabled:
		return BoolResolutionDetail{Value: true, Reason: ReasonEnabled}
	case FlagStateDisabled:
		return BoolResolutionDetail{Value: false, Reason: ReasonDisabled}
	case FlagStatePartiallyEnabled:
		for _, id := range val.EnabledList {
			if id == evalCtx.TenantID {
				return BoolResolutionDetail{Value: true, Reason: ReasonPartiallyEnabled}
			}
		}
		return BoolResolutionDetail{Value: false, Reason: ReasonPartiallyEnabled}
	default:
		return BoolResolutionDetail{Value: false, Reason: ReasonDisabled}
	}
}
