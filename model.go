package featureflags

// FlagState represents the state of a flag in an environment.
type FlagState string

const (
	FlagStateEnabled          FlagState = "enabled"
	FlagStateDisabled         FlagState = "disabled"
	FlagStatePartiallyEnabled FlagState = "partially_enabled"
)

// FlagValue is the data stored in the SDK's local cache per flag.
type FlagValue struct {
	State       FlagState `json:"state"`
	EnabledList []string  `json:"enabled_list"`
}

// EvaluationContext carries identity information for flag evaluation.
// TenantID can be a user ID, org ID, account ID, or any entity identifier in your product.
// UUID format is recommended.
type EvaluationContext struct {
	TenantID   string
	Attributes map[string]string
}

// EvaluationReason describes why a flag was resolved to its value.
type EvaluationReason string

const (
	ReasonEnabled          EvaluationReason = "ENABLED"
	ReasonDisabled         EvaluationReason = "DISABLED"
	ReasonPartiallyEnabled EvaluationReason = "PARTIALLY_ENABLED"
	ReasonFlagNotFound     EvaluationReason = "FLAG_NOT_FOUND"
	ReasonDefault          EvaluationReason = "DEFAULT"
)

// BoolResolutionDetail is the result of resolving a boolean flag.
type BoolResolutionDetail struct {
	Value  bool
	Reason EvaluationReason
}

// Subscription represents an active OnChange subscription.
// Call Unsubscribe to stop receiving callbacks.
type Subscription struct {
	unsubscribe func()
}

// Unsubscribe stops the subscription from receiving further callbacks.
func (s *Subscription) Unsubscribe() {
	s.unsubscribe()
}
