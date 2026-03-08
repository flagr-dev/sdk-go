package featureflags

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client maintains a local in-memory cache of flag values for one environment,
// seeded via the SSE init event and kept up to date via flag_update events.
type Client struct {
	sdkKey  string
	baseURL string

	mu    sync.RWMutex
	cache map[string]FlagValue

	subMu       sync.Mutex
	subscribers map[string]map[int]func(FlagValue)
	subCounter  int
}

// NewClient creates a Client. Call Connect to start the SSE stream.
// baseURL is optional — defaults to "https://api.flagr.dev".
func NewClient(sdkKey string, baseURL ...string) *Client {
	url := "https://api.flagr.dev"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	}
	return &Client{
		sdkKey:      sdkKey,
		baseURL:     strings.TrimRight(url, "/"),
		cache:       make(map[string]FlagValue),
		subscribers: make(map[string]map[int]func(FlagValue)),
	}
}

// Connect opens the SSE stream, populates the cache from the init snapshot,
// and starts a background goroutine to apply updates. It blocks until the
// init snapshot has been received or ctx is cancelled.
func (c *Client) Connect(ctx context.Context) error {
	initDone := make(chan error, 1)
	go c.stream(ctx, initDone)
	select {
	case err := <-initDone:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) stream(ctx context.Context, initDone chan<- error) {
	for {
		if err := c.connectOnce(ctx, initDone); err != nil {
			// If context is cancelled, exit cleanly.
			if ctx.Err() != nil {
				return
			}
			// Otherwise reconnect after a delay.
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		// If initDone has already been sent, don't send again.
		initDone = nil
	}
}

func (c *Client) connectOnce(ctx context.Context, initDone chan<- error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/evaluate/stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.sdkKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if initDone != nil {
			initDone <- err
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		err := fmt.Errorf("flagr: invalid SDK key — check your sdkKey and try again")
		if initDone != nil {
			initDone <- err
		}
		return nil // do not reconnect on auth failure
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("flagr: SSE stream returned HTTP %d", resp.StatusCode)
		if initDone != nil {
			initDone <- err
		}
		return err
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string
	initSent := false

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "":
			switch eventType {
			case "init":
				var allFlags map[string]FlagValue
				if err := json.Unmarshal([]byte(data), &allFlags); err == nil {
					var changed []struct {
						key string
						val FlagValue
					}
					c.mu.Lock()
					if initDone == nil {
						// Reconnect — collect flags whose value changed while disconnected.
						for k, newVal := range allFlags {
							if oldVal, existed := c.cache[k]; !existed || !flagValueEqual(oldVal, newVal) {
								changed = append(changed, struct {
									key string
									val FlagValue
								}{k, newVal})
							}
						}
					}
					for k, v := range allFlags {
						c.cache[k] = v
					}
					c.mu.Unlock()
					for _, ch := range changed {
						c.notifySubscribers(ch.key, ch.val)
					}
				}
				if initDone != nil && !initSent {
					initSent = true
					initDone <- nil
				}

			case "flag_update":
				var msg struct {
					FlagKey     string    `json:"flag_key"`
					State       FlagState `json:"state"`
					EnabledList []string  `json:"enabled_list"`
				}
				if err := json.Unmarshal([]byte(data), &msg); err == nil {
					newVal := FlagValue{State: msg.State, EnabledList: msg.EnabledList}
					c.mu.Lock()
					c.cache[msg.FlagKey] = newVal
					c.mu.Unlock()
					c.notifySubscribers(msg.FlagKey, newVal)
				}
			}
			eventType, data = "", ""
		}
	}

	return scanner.Err()
}

// GetFlagValue returns the cached value for flagKey.
// Returns (FlagValue{}, false) if the flag is not in the cache.
func (c *Client) GetFlagValue(flagKey string) (FlagValue, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.cache[flagKey]
	return v, ok
}

// subscribe registers a callback that is called with the new FlagValue whenever
// a flag_update SSE event is received for flagKey. Returns an unsubscribe func.
func (c *Client) subscribe(flagKey string, cb func(FlagValue)) func() {
	c.subMu.Lock()
	if c.subscribers[flagKey] == nil {
		c.subscribers[flagKey] = make(map[int]func(FlagValue))
	}
	id := c.subCounter
	c.subCounter++
	c.subscribers[flagKey][id] = cb
	c.subMu.Unlock()
	return func() {
		c.subMu.Lock()
		delete(c.subscribers[flagKey], id)
		c.subMu.Unlock()
	}
}

func flagValueEqual(a, b FlagValue) bool {
	if a.State != b.State || len(a.EnabledList) != len(b.EnabledList) {
		return false
	}
	for i := range a.EnabledList {
		if a.EnabledList[i] != b.EnabledList[i] {
			return false
		}
	}
	return true
}

func (c *Client) notifySubscribers(flagKey string, val FlagValue) {
	c.subMu.Lock()
	subs := c.subscribers[flagKey]
	callbacks := make([]func(FlagValue), 0, len(subs))
	for _, cb := range subs {
		callbacks = append(callbacks, cb)
	}
	c.subMu.Unlock()
	for _, cb := range callbacks {
		cb(val)
	}
}
