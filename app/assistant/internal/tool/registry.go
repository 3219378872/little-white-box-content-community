package tool

import (
	"context"
	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
	"fmt"
	"reflect"
	"strings"
)

func NewRegistry(clients Clients, allowed []string) (*Registry, error) {
	defs := allDefinitions(clients)
	reg := &Registry{
		executors: make(map[string]executorFunc, len(defs)),
		preparers: map[string]prepareFunc{},
		metadata:  map[string]Metadata{},
		allowed:   map[string]struct{}{},
		store:     clients.Store,
	}
	for _, def := range defs {
		reg.executors[def.Name] = def.executor
		reg.metadata[def.Name] = def.Metadata
		if def.prepare != nil {
			reg.preparers[def.Name] = def.prepare
		}
		if len(allowed) == 0 {
			reg.allowed[def.Name] = struct{}{}
			continue
		}
		for _, name := range allowed {
			if strings.TrimSpace(name) == def.Name {
				reg.allowed[def.Name] = struct{}{}
			}
		}
	}
	if len(reg.allowed) == 0 {
		return nil, fmt.Errorf("agent: AllowedTools must contain at least one tool")
	}
	reg.definitions = defs
	return reg, nil
}

func (r *Registry) Restrict(names []string) *Registry {
	if r == nil {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, name := range names {
		if r.Has(name) {
			allowed[name] = struct{}{}
		}
	}
	return &Registry{
		definitions: r.definitions, frozenDefinitions: filterFrozenDefinitions(r.frozenDefinitions, allowed),
		frozen: r.frozen, executors: r.executors, preparers: r.preparers, metadata: r.metadata, allowed: allowed, store: r.store,
	}
}

func (r *Registry) ResolveDefinitions(defs []prompt.ToolDef) *Registry {
	if r == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) != "" {
			allowed[def.Name] = struct{}{}
		}
	}
	return &Registry{
		definitions: r.definitions, frozenDefinitions: append([]prompt.ToolDef(nil), defs...),
		frozen: true, executors: r.executors, preparers: r.preparers, metadata: r.metadata, allowed: allowed, store: r.store,
	}
}

func (r *Registry) Prepare(ctx context.Context, session *Session, name, argsJSON string) (string, error) {
	argsJSON = canonical.UnwrapArgsJSON(argsJSON)
	if r == nil || !r.Has(name) || !r.currentlyAuthorized(session, name) {
		return "", errx.New(errx.PermissionDenied, "agent tool is not allowed")
	}
	canonicalArgs, err := r.validateArguments(name, argsJSON)
	if err != nil {
		return "", errx.New(errx.ParamError, "tool arguments are invalid")
	}
	prepare := r.preparers[name]
	if prepare == nil {
		return canonicalArgs, nil
	}
	return prepare(ctx, session, canonicalArgs)
}

func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.allowed[name]
	return ok
}

func (r *Registry) HighRisk(name string) bool {
	if r == nil {
		return false
	}
	current := r.metadata[name].Confirmation
	frozen, ok := r.frozenMetadata(name)
	return current || (ok && frozen.Confirmation)
}

func (r *Registry) SideEffect(name string) bool {
	if r == nil {
		return false
	}
	current := r.metadata[name].Effect == EffectWrite
	frozen, ok := r.frozenMetadata(name)
	return current || (ok && frozen.Effect == EffectWrite)
}

func (r *Registry) Poller(name string) bool {
	if r == nil || !r.metadata[name].Poller {
		return false
	}
	frozen, ok := r.frozenMetadata(name)
	return !ok || frozen.Poller
}

func (r *Registry) Metadata(name string) (Metadata, bool) {
	if r == nil {
		return Metadata{}, false
	}
	meta, ok := r.metadata[name]
	return meta, ok
}

func (r *Registry) Definitions() []prompt.ToolDef {
	if r == nil {
		return nil
	}
	if r.frozen {
		return filterFrozenDefinitions(r.frozenDefinitions, r.allowed)
	}
	out := make([]prompt.ToolDef, 0)
	for _, def := range r.definitions {
		if _, ok := r.allowed[def.Name]; !ok {
			continue
		}
		meta := def.Metadata
		if !meta.Available {
			continue
		}
		out = append(out, prompt.ToolDef{
			MinClientProtocol: meta.MinClientProtocol,
			Name:              def.Name, Description: def.Description, Parameters: def.Parameters,
			Effect: meta.Effect, Sources: append([]string(nil), meta.Sources...), MinConsent: meta.MinConsent,
			Confirmation: meta.Confirmation, Idempotency: meta.Idempotency,
			MaxResultBytes: meta.MaxResultBytes, Poller: meta.Poller,
		})
	}
	return out
}

func (r *Registry) Call(ctx context.Context, session *Session, name, callID, argsJSON string) (string, []store.SourceRef, error) {
	if !r.Has(name) || !r.currentlyAuthorized(session, name) {
		return "", nil, errx.New(errx.PermissionDenied, "agent tool is not allowed")
	}
	if _, err := r.validateArguments(name, argsJSON); err != nil {
		return "", nil, errx.New(errx.ParamError, "tool arguments are invalid")
	}
	meta, metaOK := r.metadata[name]
	handle, ok := r.executors[name]
	if !ok || !metaOK || !meta.Available || handle == nil {
		return unavailableResult(name), nil, &UnavailableError{Tool: name}
	}
	text, sources, err := handle(ctx, session, callID, argsJSON)
	if err != nil {
		return "", nil, err
	}
	limit := r.resultLimit(name, meta.MaxResultBytes)
	if name == PresentSources {
		return limitResult(text, limit), sources, nil
	}
	if len(sources) > 0 {
		if r.store == nil || session == nil || session.RunID <= 0 {
			return "", nil, errx.New(errx.ServiceUnavailable, "source ledger unavailable")
		}
		text, err = r.bindSources(ctx, session, sources, text)
		if err != nil {
			return "", nil, err
		}
	}
	return limitResult(text, limit), nil, nil
}

// validateArguments applies the frozen tool schema before any executor sees
// model-controlled JSON. Executors still decode into their concrete structs,
// but this central gate guarantees unknown fields, trailing values and basic
// type mismatches are handled consistently for every tool.
func (r *Registry) validateArguments(name, raw string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("nil tool registry")
	}
	raw = canonical.UnwrapArgsJSON(raw)
	if raw == "" {
		raw = "{}"
	}
	value, err := decodeStrictValue(raw)
	if err != nil {
		return "", err
	}
	definition, ok := r.definition(name)
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if err := validateSchemaValue(value, definition.Parameters, "$"); err != nil {
		return "", err
	}
	// A recovered run uses the schema captured in its prompt epoch. Validate
	// against the current definition as well so removed fields cannot silently
	// reach a newer executor implementation.
	for _, frozen := range r.frozenDefinitions {
		if frozen.Name != name || reflect.DeepEqual(frozen.Parameters, definition.Parameters) {
			continue
		}
		if err := validateSchemaValue(value, frozen.Parameters, "$"); err != nil {
			return "", err
		}
	}
	canonicalValue, err := canonical.JSON(value)
	if err != nil {
		return "", err
	}
	return string(canonicalValue), nil
}

func (r *Registry) definition(name string) (Definition, bool) {
	for _, definition := range r.definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}
