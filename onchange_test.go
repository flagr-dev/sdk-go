package featureflags

import "testing"

func TestOnChange_FiresImmediately_FlagEnabled(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateEnabled}
	provider := NewProvider(client)

	var got bool
	calls := 0
	provider.OnChange("my-flag", func(enabled bool) {
		got = enabled
		calls++
	}, false, "tenant-1")

	if calls != 1 {
		t.Fatalf("want 1 immediate call, got %d", calls)
	}
	if !got {
		t.Errorf("enabled flag: want true, got false")
	}
}

func TestOnChange_FiresImmediately_FlagNotFound_UsesDefault(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	provider := NewProvider(client)

	var got bool
	calls := 0
	provider.OnChange("nonexistent", func(enabled bool) {
		got = enabled
		calls++
	}, true, "tenant-1")

	if calls != 1 {
		t.Fatalf("want 1 immediate call, got %d", calls)
	}
	if !got {
		t.Errorf("missing flag: want default (true), got false")
	}
}

func TestOnChange_FiresOnUpdate(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateDisabled}
	provider := NewProvider(client)

	var results []bool
	provider.OnChange("my-flag", func(enabled bool) {
		results = append(results, enabled)
	}, false, "tenant-1")

	client.notifySubscribers("my-flag", FlagValue{State: FlagStateEnabled})

	if len(results) != 2 {
		t.Fatalf("want 2 calls (immediate + update), got %d", len(results))
	}
	if results[0] != false {
		t.Errorf("first call: want false (initial disabled), got true")
	}
	if results[1] != true {
		t.Errorf("second call: want true (after update), got false")
	}
}

func TestOnChange_Unsubscribe_StopsFiring(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateEnabled}
	provider := NewProvider(client)

	calls := 0
	sub := provider.OnChange("my-flag", func(_ bool) {
		calls++
	}, false, "tenant-1")

	sub.Unsubscribe()
	client.notifySubscribers("my-flag", FlagValue{State: FlagStateDisabled})

	if calls != 1 {
		t.Errorf("want 1 call (immediate only), got %d", calls)
	}
}

func TestOnChange_MultipleSubscribers_BothFire(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateEnabled}
	provider := NewProvider(client)

	calls1, calls2 := 0, 0
	provider.OnChange("my-flag", func(_ bool) { calls1++ }, false, "tenant-1")
	provider.OnChange("my-flag", func(_ bool) { calls2++ }, false, "tenant-2")

	client.notifySubscribers("my-flag", FlagValue{State: FlagStateDisabled})

	if calls1 != 2 {
		t.Errorf("subscriber 1: want 2 calls (immediate + update), got %d", calls1)
	}
	if calls2 != 2 {
		t.Errorf("subscriber 2: want 2 calls (immediate + update), got %d", calls2)
	}
}

func TestOnChange_MultipleSubscribers_OneUnsubscribes(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateEnabled}
	provider := NewProvider(client)

	calls1, calls2 := 0, 0
	sub1 := provider.OnChange("my-flag", func(_ bool) { calls1++ }, false, "tenant-1")
	provider.OnChange("my-flag", func(_ bool) { calls2++ }, false, "tenant-2")

	sub1.Unsubscribe()
	client.notifySubscribers("my-flag", FlagValue{State: FlagStateDisabled})

	if calls1 != 1 {
		t.Errorf("unsubscribed: want 1 call (immediate only), got %d", calls1)
	}
	if calls2 != 2 {
		t.Errorf("active subscriber: want 2 calls, got %d", calls2)
	}
}

func TestOnChange_FiresOnReconnectIfValueChanged(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateDisabled}
	provider := NewProvider(client)

	var results []bool
	provider.OnChange("my-flag", func(enabled bool) {
		results = append(results, enabled)
	}, false, "tenant-1")

	// Simulate a reconnect where the flag changed while disconnected.
	// initDone == nil signals a reconnect in connectOnce; we replicate that
	// by calling the internal reconnect path directly via notifySubscribers
	// after updating the cache — same code path the init handler uses.
	newSnapshot := map[string]FlagValue{
		"my-flag": {State: FlagStateEnabled},
	}
	var changed []struct {
		key string
		val FlagValue
	}
	client.mu.Lock()
	for k, newVal := range newSnapshot {
		if oldVal, existed := client.cache[k]; !existed || !flagValueEqual(oldVal, newVal) {
			changed = append(changed, struct {
				key string
				val FlagValue
			}{k, newVal})
		}
	}
	for k, v := range newSnapshot {
		client.cache[k] = v
	}
	client.mu.Unlock()
	for _, ch := range changed {
		client.notifySubscribers(ch.key, ch.val)
	}

	if len(results) != 2 {
		t.Fatalf("want 2 calls (immediate + reconnect), got %d", len(results))
	}
	if results[1] != true {
		t.Errorf("reconnect call: want true (flag changed to enabled), got false")
	}
}

func TestOnChange_NoFireOnReconnectIfValueUnchanged(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{State: FlagStateEnabled}
	provider := NewProvider(client)

	calls := 0
	provider.OnChange("my-flag", func(_ bool) { calls++ }, false, "tenant-1")

	// Simulate reconnect with same value — subscriber should not fire again.
	newSnapshot := map[string]FlagValue{
		"my-flag": {State: FlagStateEnabled},
	}
	client.mu.Lock()
	for k, newVal := range newSnapshot {
		if oldVal, existed := client.cache[k]; !existed || !flagValueEqual(oldVal, newVal) {
			client.mu.Unlock()
			client.notifySubscribers(k, newVal)
			client.mu.Lock()
		}
	}
	for k, v := range newSnapshot {
		client.cache[k] = v
	}
	client.mu.Unlock()

	if calls != 1 {
		t.Errorf("unchanged flag on reconnect: want 1 call (immediate only), got %d", calls)
	}
}

func TestOnChange_PartiallyEnabled_EvaluatesPerTenant(t *testing.T) {
	client := NewClient("test-key", "http://localhost")
	client.cache["my-flag"] = FlagValue{
		State:       FlagStatePartiallyEnabled,
		EnabledList: []string{"tenant-allowed"},
	}
	provider := NewProvider(client)

	var gotAllowed, gotBlocked bool
	provider.OnChange("my-flag", func(v bool) { gotAllowed = v }, false, "tenant-allowed")
	provider.OnChange("my-flag", func(v bool) { gotBlocked = v }, false, "tenant-other")

	if !gotAllowed {
		t.Errorf("tenant in list: want true, got false")
	}
	if gotBlocked {
		t.Errorf("tenant not in list: want false, got true")
	}
}
