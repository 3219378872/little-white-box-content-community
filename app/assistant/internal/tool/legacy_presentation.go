package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/assistant/internal/store"
)

// Legacy events prove run-level provenance, never sentence-level associations.
func LegacyPresentation(ctx context.Context, st store.Store, message store.Message) (*store.AnswerPresentation, error) {
	if message.RunID <= 0 {
		return nil, nil
	}
	events, err := st.ListSourceEvents(ctx, message.RunID)
	if err != nil {
		return nil, err
	}
	answer := &store.AnswerPresentation{Version: 1, MessageID: message.ID, RunID: message.RunID, Blocks: []store.AnswerBlock{{ID: "legacy", Kind: "context", Text: message.Content, Citations: []store.AnswerCitation{}}}, Sources: []store.ResearchSource{}}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != store.EventSourceCard {
			continue
		}
		var payload store.EventPayload
		if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
			return nil, err
		}
		ref := payload.SourceCard
		if ref == nil || ref.Handle == "" {
			continue
		}
		identity := fmt.Sprintf("%s/%s/%d", ref.Kind, ref.AuthorityID, ref.Revision)
		if seen[identity] || len(answer.Sources) >= 10 {
			continue
		}
		seen[identity] = true
		card := store.ResearchSource{Handle: ref.Handle, Kind: ref.Kind, AuthorityID: ref.AuthorityID, Title: ref.Title, Revision: ref.Revision, Available: true, Excerpts: []store.Evidence{}}
		if ref.Kind == "post" {
			card.URL = "/post/" + ref.AuthorityID
		} else if ref.Kind == "web" && SafeSourceURL(ref.AuthorityID) {
			card.URL = ref.AuthorityID
		} else {
			continue
		}
		var data struct {
			Snippet string `json:"snippet"`
		}
		if json.Unmarshal([]byte(ref.PayloadJSON), &data) == nil && data.Snippet != "" {
			card.Excerpts = append(card.Excerpts, store.Evidence{ID: "legacy-" + ref.Handle, Handle: ref.Handle, Kind: ref.Kind, Text: data.Snippet, RetrievedAtMs: event.CreatedAtMs})
		}
		answer.Sources = append(answer.Sources, card)
	}
	if len(answer.Sources) == 0 {
		return nil, nil
	}
	return answer, nil
}
