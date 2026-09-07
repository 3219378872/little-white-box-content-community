package runtime

import (
	"context"
	"errors"
	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"strings"
)

func (e *Engine) loadCapabilities(ctx context.Context, run store.Run, session *store.Session) (*tool.Registry, llm.Client, error) {
	if session == nil {
		return nil, nil, errors.New("assistant session is nil")
	}
	capability, ok := prompt.DecodeCapabilities(session.ToolSnapshot)
	changed := false
	if !ok {
		var buildErr error
		capability, buildErr = e.buildCapabilitySnapshot(run)
		if buildErr != nil {
			return nil, nil, e.fail(ctx, run, "TOOLS_UNAVAILABLE", buildErr.Error())
		}
		changed = true
	} else if capability.Version == 0 {
		capability.Version = prompt.CapabilitySnapshotVersion
		capability.Provider = llm.Capability(e.LLM)
		changed = true
	}
	if !capability.Provider.Tools || strings.TrimSpace(capability.Provider.RouteID) == "" {
		return nil, nil, e.fail(ctx, run, "PROVIDER_CAPABILITY_UNAVAILABLE", "frozen provider route is unavailable")
	}
	base := e.Tools.ResolveDefinitions(capability.Tools)
	registry := tool.ForClient(tool.ForSource(base, run.Source, run.ConsentVersion), clientProtocol(run))
	if registry == nil || (run.Source == store.SourceMemoryReview && len(registry.Definitions()) == 0) {
		return nil, nil, e.fail(ctx, run, "TOOLS_UNAVAILABLE", "no frozen tools for run source")
	}
	client, routeOK := llm.SelectCapability(e.LLM, capability.Provider)
	if !routeOK {
		return nil, nil, e.fail(ctx, run, "PROVIDER_ROUTE_UNAVAILABLE", "frozen provider route is unavailable")
	}
	if changed {
		session.ToolSnapshot = prompt.EncodeCapabilities(capability)
		if err := e.step(ctx, run, func(ctx context.Context, tx store.Store) error {
			return tx.UpdateSession(ctx, *session)
		}); err != nil {
			return nil, nil, err
		}
	}
	return registry, client, nil
}

func (e *Engine) buildCapabilitySnapshot(run store.Run) (prompt.CapabilitySnapshot, error) {
	base := tool.ForSource(e.Tools, store.SourceUser, run.ConsentVersion)
	if base == nil {
		return prompt.CapabilitySnapshot{}, errors.New("no available tools")
	}
	provider := llm.Capability(e.LLM)
	if !provider.Tools || strings.TrimSpace(provider.RouteID) == "" {
		return prompt.CapabilitySnapshot{}, errors.New("provider capability is unavailable")
	}
	return prompt.CapabilitySnapshot{
		Version:  prompt.CapabilitySnapshotVersion,
		Tools:    base.Definitions(),
		Provider: provider,
	}, nil
}
