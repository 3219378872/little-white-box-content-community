package tool

import (
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"strings"
)

func RestrictToolsForConsent(registry *Registry, consentVersion int32) *Registry {
	if registry == nil || consentVersion >= CurrentConsentVersion {
		return registry
	}
	return registry.Restrict(Version1Tools())
}

func ForSource(registry *Registry, source string, consentVersion int32) *Registry {
	if registry == nil {
		return nil
	}
	if registry.frozen {
		names := make([]string, 0, len(registry.frozenDefinitions))
		for _, def := range registry.frozenDefinitions {
			if _, configured := registry.allowed[def.Name]; !configured {
				continue
			}
			if len(def.Sources) == 0 {
				meta, ok := registry.metadata[def.Name]
				if ok && consentVersion >= meta.MinConsent && containsString(meta.Sources, source) {
					names = append(names, def.Name)
				}
				continue
			}
			if consentVersion >= def.MinConsent && containsString(def.Sources, source) {
				names = append(names, def.Name)
			}
		}
		return registry.Restrict(names)
	}
	names := make([]string, 0, len(registry.metadata))
	for name, meta := range registry.metadata {
		if !meta.Available || consentVersion < meta.MinConsent || !containsString(meta.Sources, source) {
			continue
		}
		if _, configured := registry.allowed[name]; configured {
			names = append(names, name)
		}
	}
	return registry.Restrict(names)
}

func (r *Registry) currentlyAuthorized(session *Session, name string) bool {
	if r == nil {
		return false
	}
	meta, ok := r.metadata[name]
	if !ok {
		return false
	}
	if meta.MinClientProtocol > 0 && (session == nil || session.ClientProtocolVersion < meta.MinClientProtocol) {
		return false
	}
	source := store.SourceUser
	consent := CurrentConsentVersion
	if session != nil {
		if strings.TrimSpace(session.Source) != "" {
			source = session.Source
		}
		if session.ConsentVersion > 0 {
			consent = session.ConsentVersion
		}
	}
	return consent >= meta.MinConsent && containsString(meta.Sources, source)
}

func (r *Registry) frozenMetadata(name string) (Metadata, bool) {
	if r == nil || !r.frozen {
		return Metadata{}, false
	}
	for _, def := range r.frozenDefinitions {
		if def.Name != name {
			continue
		}
		return Metadata{
			MinClientProtocol: def.MinClientProtocol,
			Effect:            def.Effect, Sources: append([]string(nil), def.Sources...), MinConsent: def.MinConsent,
			Confirmation: def.Confirmation, Idempotency: def.Idempotency,
			MaxResultBytes: def.MaxResultBytes, Poller: def.Poller,
		}, true
	}
	return Metadata{}, false
}

func (r *Registry) resultLimit(name string, current int) int {
	frozen, ok := r.frozenMetadata(name)
	if !ok || frozen.MaxResultBytes <= 0 {
		return current
	}
	if current <= 0 || frozen.MaxResultBytes < current {
		return frozen.MaxResultBytes
	}
	return current
}

func filterFrozenDefinitions(defs []prompt.ToolDef, allowed map[string]struct{}) []prompt.ToolDef {
	out := make([]prompt.ToolDef, 0, len(defs))
	for _, def := range defs {
		if _, ok := allowed[def.Name]; ok {
			out = append(out, def)
		}
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
