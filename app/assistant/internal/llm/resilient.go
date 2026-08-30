package llm

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"esx/app/assistant/internal/prompt"
)

const (
	AttemptStart   = "start"
	AttemptReset   = "reset"
	AttemptError   = "error"
	AttemptSuccess = "success"
)

type AttemptEvent struct {
	Kind       string
	Attempt    int
	RouteID    string
	StreamID   string
	ErrorKind  ErrorKind
	StatusCode int
	Retryable  bool
}

type AttemptObserver func(AttemptEvent) error

type Delta struct {
	Text string
}

type StreamingClient interface {
	CompleteStream(ctx context.Context, req Request, emit func(Delta) error) (Result, error)
}

type Route struct {
	ID       string
	Boundary string
	Client   Client
}

type RetryOptions struct {
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	MaxRetryAfter time.Duration
	Sleep         func(context.Context, time.Duration) error
	Rand          *rand.Rand
}

type ResilientClient struct {
	routes   []Route
	options  RetryOptions
	randomMu *sync.Mutex
}

var defaultRandomMu sync.Mutex

func NewResilient(routes []Route, options RetryOptions) (*ResilientClient, error) {
	clean := make([]Route, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		route.ID = strings.TrimSpace(route.ID)
		if route.Client == nil || route.ID == "" {
			continue
		}
		if _, exists := seen[route.ID]; exists {
			return nil, fmt.Errorf("duplicate assistant LLM route %q", route.ID)
		}
		seen[route.ID] = struct{}{}
		clean = append(clean, route)
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("assistant LLM requires at least one route")
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 3
	}
	if options.BaseDelay <= 0 {
		options.BaseDelay = 250 * time.Millisecond
	}
	if options.MaxDelay <= 0 {
		options.MaxDelay = 5 * time.Second
	}
	if options.MaxRetryAfter <= 0 {
		options.MaxRetryAfter = 30 * time.Second
	}
	if options.Sleep == nil {
		options.Sleep = sleepContext
	}
	if options.Rand == nil {
		options.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &ResilientClient{routes: clean, options: options, randomMu: &sync.Mutex{}}, nil
}

func (c *ResilientClient) Complete(ctx context.Context, req Request) (Result, error) {
	return c.complete(ctx, req, nil)
}

func (c *ResilientClient) CompleteStream(ctx context.Context, req Request, emit func(Delta) error) (Result, error) {
	return c.complete(ctx, req, emit)
}

func (c *ResilientClient) complete(ctx context.Context, req Request, emit func(Delta) error) (Result, error) {
	routes := []Route{c.routes[0]}
	for _, route := range c.routes[1:] {
		if compatibleRoute(c.routes[0], route, emit != nil) {
			routes = append(routes, route)
		}
	}
	var last error
	for attempt := 1; attempt <= c.options.MaxAttempts; attempt++ {
		route := routes[0]
		if len(routes) > 1 && attempt == c.options.MaxAttempts {
			route = routes[1]
		}
		streamID := streamIDFor(req.AttemptPrefix, route.ID, attempt)
		if err := observe(req.Observer, AttemptEvent{Kind: AttemptStart, Attempt: attempt, RouteID: route.ID, StreamID: streamID}); err != nil {
			return Result{}, err
		}
		emitted := false
		wrappedEmit := func(delta Delta) error {
			if delta.Text != "" {
				emitted = true
			}
			if emit == nil {
				return nil
			}
			return emit(delta)
		}
		var result Result
		var err error
		if emit != nil {
			if stream, ok := route.Client.(StreamingClient); ok {
				result, err = stream.CompleteStream(ctx, req, wrappedEmit)
			} else {
				result, err = route.Client.Complete(ctx, req)
				if err == nil && result.Text != "" {
					err = wrappedEmit(Delta{Text: result.Text})
				}
			}
		} else {
			result, err = route.Client.Complete(ctx, req)
		}
		if err == nil {
			result.StreamID = streamID
			result.Attempts = attempt
			if observerErr := observe(req.Observer, AttemptEvent{Kind: AttemptSuccess, Attempt: attempt, RouteID: route.ID, StreamID: streamID}); observerErr != nil {
				return Result{}, observerErr
			}
			return result, nil
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		classified := ClassifyError(err)
		last = classified
		if observerErr := observe(req.Observer, AttemptEvent{
			Kind: AttemptError, Attempt: attempt, RouteID: route.ID, StreamID: streamID,
			ErrorKind: classified.Kind, StatusCode: classified.StatusCode, Retryable: classified.Retryable,
		}); observerErr != nil {
			return Result{}, observerErr
		}
		hasNext := classified.Retryable && attempt < c.options.MaxAttempts
		if emitted && hasNext {
			if observerErr := observe(req.Observer, AttemptEvent{Kind: AttemptReset, Attempt: attempt, RouteID: route.ID, StreamID: streamID}); observerErr != nil {
				return Result{}, observerErr
			}
		}
		if !hasNext {
			break
		}
		if err := c.options.Sleep(ctx, c.retryDelay(attempt, classified.RetryAfter)); err != nil {
			return Result{}, err
		}
	}
	if last == nil {
		last = &ProviderError{Kind: ErrorUnknown, Message: "assistant LLM routes are unavailable"}
	}
	return Result{}, last
}

func (c *ResilientClient) retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	delay := c.options.BaseDelay
	for i := 1; i < attempt && delay < c.options.MaxDelay; i++ {
		delay *= 2
	}
	if delay > c.options.MaxDelay {
		delay = c.options.MaxDelay
	}
	randomMu := c.randomMu
	if randomMu == nil {
		randomMu = &defaultRandomMu
	}
	randomMu.Lock()
	jitter := time.Duration(c.options.Rand.Int63n(maxDuration(delay/2, time.Nanosecond).Nanoseconds()))
	randomMu.Unlock()
	delay += jitter
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > c.options.MaxRetryAfter {
		delay = c.options.MaxRetryAfter
	}
	return delay
}

func (c *ResilientClient) SupportsTools() bool      { return c != nil && c.routes[0].Client.SupportsTools() }
func (c *ResilientClient) WireAPI() string          { return c.routes[0].Client.WireAPI() }
func (c *ResilientClient) MaxOutputTokens() int     { return c.routes[0].Client.MaxOutputTokens() }
func (c *ResilientClient) ContextWindowTokens() int { return c.routes[0].Client.ContextWindowTokens() }
func (c *ResilientClient) RouteID() string          { return c.routes[0].ID }
func (c *ResilientClient) Boundary() string         { return c.routes[0].Boundary }
func (c *ResilientClient) SupportsStreaming() bool {
	_, ok := c.routes[0].Client.(StreamingClient)
	return ok
}
func (c *ResilientClient) ModelName() string { return Capability(c.routes[0].Client).Model }
func (c *ResilientClient) FallbackRouteIDs() []string {
	out := make([]string, 0, len(c.routes)-1)
	for _, route := range c.routes[1:] {
		if compatibleRoute(c.routes[0], route, true) {
			out = append(out, route.ID)
		}
	}
	return out
}

func (c *ResilientClient) ForRoute(routeID string) (Client, bool) {
	for i, route := range c.routes {
		if route.ID != routeID {
			continue
		}
		routes := []Route{route}
		for _, fallback := range c.routes[i+1:] {
			if compatibleRoute(route, fallback, true) {
				routes = append(routes, fallback)
			}
		}
		return &ResilientClient{routes: routes, options: c.options, randomMu: c.randomMu}, true
	}
	return nil, false
}

func (c *ResilientClient) ExactRoute(routeID string) (Client, bool) {
	for _, route := range c.routes {
		if route.ID != routeID {
			continue
		}
		return &ResilientClient{routes: []Route{route}, options: c.options, randomMu: c.randomMu}, true
	}
	return nil, false
}

func (c *ResilientClient) ForCapability(capability prompt.ProviderCapability) (Client, bool) {
	var primary Route
	found := false
	for _, route := range c.routes {
		if route.ID == capability.RouteID {
			primary = route
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}
	routes := []Route{primary}
	seen := map[string]struct{}{primary.ID: {}}
	for _, routeID := range capability.FallbackRouteIDs {
		if _, duplicate := seen[routeID]; duplicate {
			continue
		}
		for _, candidate := range c.routes {
			if candidate.ID != routeID || !compatibleRoute(primary, candidate, capability.Streaming) {
				continue
			}
			routes = append(routes, candidate)
			seen[routeID] = struct{}{}
			break
		}
	}
	return &ResilientClient{routes: routes, options: c.options, randomMu: c.randomMu}, true
}

func compatibleRoute(primary, fallback Route, requireStreaming bool) bool {
	if primary.Client == nil || fallback.Client == nil || !fallback.Client.SupportsTools() {
		return false
	}
	if requireStreaming {
		if _, ok := fallback.Client.(StreamingClient); !ok {
			return false
		}
	}
	if fallback.Client.ContextWindowTokens() < primary.Client.ContextWindowTokens() ||
		fallback.Client.MaxOutputTokens() < primary.Client.MaxOutputTokens() {
		return false
	}
	return strings.TrimSpace(primary.Boundary) == strings.TrimSpace(fallback.Boundary)
}

func streamIDFor(prefix, route string, attempt int) string {
	if prefix == "" {
		prefix = "stream"
	}
	return fmt.Sprintf("%s-%s-%d", prefix, route, attempt)
}

func observe(observer AttemptObserver, event AttemptEvent) error {
	if observer != nil {
		return observer(event)
	}
	return nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

var _ Client = (*ResilientClient)(nil)
var _ StreamingClient = (*ResilientClient)(nil)
var _ RouteSelector = (*ResilientClient)(nil)
var _ exactRouteSelector = (*ResilientClient)(nil)
var _ capabilityRouteSelector = (*ResilientClient)(nil)
