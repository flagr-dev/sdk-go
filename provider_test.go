package featureflags

import "testing"

func TestEvaluate_Enabled(t *testing.T) {
	val := FlagValue{State: FlagStateEnabled}
	got := evaluate(val, EvaluationContext{TenantID: "any-tenant"})
	if !got.Value {
		t.Errorf("enabled flag: want true, got false")
	}
	if got.Reason != ReasonEnabled {
		t.Errorf("enabled flag: want reason %q, got %q", ReasonEnabled, got.Reason)
	}
}

func TestEvaluate_Disabled(t *testing.T) {
	val := FlagValue{State: FlagStateDisabled}
	got := evaluate(val, EvaluationContext{TenantID: "any-tenant"})
	if got.Value {
		t.Errorf("disabled flag: want false, got true")
	}
	if got.Reason != ReasonDisabled {
		t.Errorf("disabled flag: want reason %q, got %q", ReasonDisabled, got.Reason)
	}
}

func TestEvaluate_PartiallyEnabled_TenantInList(t *testing.T) {
	val := FlagValue{
		State:       FlagStatePartiallyEnabled,
		EnabledList: []string{"a1b2c3d4-0000-0000-0000-000000000001", "a1b2c3d4-0000-0000-0000-000000000002"},
	}
	got := evaluate(val, EvaluationContext{TenantID: "a1b2c3d4-0000-0000-0000-000000000001"})
	if !got.Value {
		t.Errorf("partially_enabled, tenant in list: want true, got false")
	}
}

func TestEvaluate_PartiallyEnabled_TenantNotInList(t *testing.T) {
	val := FlagValue{
		State:       FlagStatePartiallyEnabled,
		EnabledList: []string{"a1b2c3d4-0000-0000-0000-000000000001"},
	}
	got := evaluate(val, EvaluationContext{TenantID: "c3d4e5f6-0000-0000-0000-000000000099"})
	if got.Value {
		t.Errorf("partially_enabled, tenant not in list: want false, got true")
	}
}

func TestEvaluate_PartiallyEnabled_EmptyList(t *testing.T) {
	val := FlagValue{State: FlagStatePartiallyEnabled, EnabledList: []string{}}
	got := evaluate(val, EvaluationContext{TenantID: "any-tenant"})
	if got.Value {
		t.Errorf("partially_enabled, empty list: want false, got true")
	}
}

func TestProvider_FlagNotFound_ReturnsDefault(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	provider := NewProvider(client)

	got := provider.ResolveBooleanValue(nil, "nonexistent-flag", true, EvaluationContext{TenantID: "t"})
	if !got.Value {
		t.Errorf("missing flag: want default (true), got false")
	}
	if got.Reason != ReasonFlagNotFound {
		t.Errorf("missing flag: want reason %q, got %q", ReasonFlagNotFound, got.Reason)
	}
}
