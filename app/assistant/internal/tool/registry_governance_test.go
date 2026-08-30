package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func TestStrictUnmarshalRejectsUnknownAndTrailingValues(t *testing.T) {
	type args struct {
		Keyword string `json:"keyword"`
	}
	for _, raw := range []string{
		`{"keyword":"go","unknown":true}`,
		`{"keyword":"go"} {"keyword":"second"}`,
	} {
		var got args
		if err := strictUnmarshal(raw, &got); err == nil {
			t.Fatalf("strictUnmarshal(%q) succeeded", raw)
		}
	}
	var got args
	if err := strictUnmarshal(`{"keyword":"go"}`, &got); err != nil || got.Keyword != "go" {
		t.Fatalf("valid args=%+v err=%v", got, err)
	}
}

func TestRegistryPrepareEnforcesFrozenToolSchema(t *testing.T) {
	registry, err := NewRegistry(Clients{Memory: memory.NewMapStore()}, []string{GetMemory})
	if err != nil {
		t.Fatal(err)
	}
	view := ForSource(registry, store.SourceUser, CurrentConsentVersion)
	session := &Session{UserID: 1, Source: store.SourceUser, ConsentVersion: CurrentConsentVersion}
	for _, raw := range []string{
		`{"target":"memory","unexpected":true}`,
		`{"target":123}`,
		`{"target":"memory"}{"target":"user"}`,
	} {
		if _, err := view.Prepare(context.Background(), session, GetMemory, raw); !errx.Is(err, errx.ParamError) {
			t.Fatalf("Prepare(%s) error=%v, want ParamError", raw, err)
		}
	}
	if _, _, err := view.Call(context.Background(), session, GetMemory, "call-1", `{"target":"memory","unexpected":true}`); !errx.Is(err, errx.ParamError) {
		t.Fatalf("Call unknown field error=%v, want ParamError", err)
	}
}

func TestToolResultLimitIsBoundedStructuredAndUTF8Safe(t *testing.T) {
	const limit = 160
	result := limitResult(strings.Repeat("结果", 300), limit)
	if len(result) > limit || !utf8.ValidString(result) {
		t.Fatalf("result bytes=%d valid=%v", len(result), utf8.ValidString(result))
	}
	var payload struct {
		OK            bool   `json:"ok"`
		Truncated     bool   `json:"truncated"`
		OriginalBytes int    `json:"original_bytes"`
		Text          string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || !payload.Truncated || payload.OriginalBytes <= limit {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestFrozenDefinitionsIgnoreRegistryChangesButExecutionRechecksPolicy(t *testing.T) {
	registry, err := NewRegistry(Clients{Memory: memory.NewMapStore()}, []string{GetMemory})
	if err != nil {
		t.Fatal(err)
	}
	base := ForSource(registry, store.SourceUser, CurrentConsentVersion)
	definitions := base.Definitions()
	if len(definitions) != 1 {
		t.Fatalf("definitions=%+v", definitions)
	}
	definitions[0].Confirmation = true
	definitions[0].Effect = EffectWrite
	frozen := registry.ResolveDefinitions(definitions)

	current := registry.metadata[GetMemory]
	current.Sources = []string{store.SourceWatch}
	current.Confirmation = false
	current.Effect = EffectRead
	registry.metadata[GetMemory] = current

	userView := ForSource(frozen, store.SourceUser, CurrentConsentVersion)
	if !reflect.DeepEqual(userView.Definitions(), definitions) {
		t.Fatalf("frozen definitions changed: got=%+v want=%+v", userView.Definitions(), definitions)
	}
	if !userView.HighRisk(GetMemory) || !userView.SideEffect(GetMemory) {
		t.Fatal("a later metadata relaxation weakened frozen confirmation/effect")
	}
	_, err = userView.Prepare(context.Background(), &Session{
		UserID: 1, Source: store.SourceUser, ConsentVersion: CurrentConsentVersion,
	}, GetMemory, `{}`)
	if !errx.Is(err, errx.PermissionDenied) {
		t.Fatalf("current source policy was not rechecked: %v", err)
	}
}

func TestNewEpochExcludesUnavailableToolsAndCarriesMetadata(t *testing.T) {
	unavailable, err := NewRegistry(Clients{}, []string{GetMemory})
	if err != nil {
		t.Fatal(err)
	}
	if got := ForSource(unavailable, store.SourceUser, CurrentConsentVersion).Definitions(); len(got) != 0 {
		t.Fatalf("unavailable definitions=%+v", got)
	}

	available, err := NewRegistry(Clients{Memory: memory.NewMapStore()}, []string{GetMemory, AddMemory})
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range ForSource(available, store.SourceUser, CurrentConsentVersion).Definitions() {
		if def.Effect == "" || len(def.Sources) == 0 || def.MinConsent == 0 ||
			def.Idempotency == "" || def.MaxResultBytes <= 0 {
			t.Fatalf("incomplete metadata: %+v", def)
		}
	}
}
