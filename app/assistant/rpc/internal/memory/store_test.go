package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMapStoreConflictKeepsOneCurrentRecord(t *testing.T) {
	store := NewMapStore()
	ctx := context.Background()
	now := time.UnixMilli(1_700_000_000_000)
	if err := store.Apply(ctx, 2, Candidate{Layer: LayerProfile, Dimension: "topic", Value: "水文", Score: 0.8, Source: SourceConversation, Confidence: 0.9}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(ctx, 2, Candidate{Layer: LayerProfile, Dimension: "topic", Value: "水文", Score: -0.7, Source: SourceConversation, Confidence: 0.95}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, 2, LayerProfile, now.Add(time.Hour))
	if err != nil || len(items) != 1 {
		t.Fatalf("got %d err=%v", len(items), err)
	}
	if items[0].Score != -0.7 {
		t.Fatalf("score=%v", items[0].Score)
	}
}

func TestInterestDecayDropsBelowFloorInContext(t *testing.T) {
	store := NewMapStore()
	ctx := context.Background()
	then := time.UnixMilli(1_700_000_000_000)
	if err := store.Apply(ctx, 2, Candidate{Layer: LayerInterest, Dimension: "topic", Value: "fifa", Score: 0.6, Source: SourceConversation, Confidence: 0.7}, then); err != nil {
		t.Fatal(err)
	}
	later := then.Add(200 * 24 * time.Hour)
	block, err := store.ContextBlock(ctx, 2, "recommend", later, false)
	if err != nil {
		t.Fatal(err)
	}
	if block != "" {
		t.Fatalf("decayed interest must not constrain recommend: %q", block)
	}
}

func TestExtractExplicitPreferences(t *testing.T) {
	got := Extract("我不喜欢水文")
	if len(got) != 1 || got[0].Score >= 0 || got[0].Value != "水文" {
		t.Fatalf("%+v", got)
	}
	got = Extract("帮我找周末能看完的攻略")
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestExtractAndApplyRejectPrivateValues(t *testing.T) {
	if got := Extract("我喜欢13800138000"); len(got) != 0 {
		t.Fatalf("phone extracted: %+v", got)
	}
	if got := Extract("我喜欢验证码"); len(got) != 0 {
		t.Fatalf("verify code extracted: %+v", got)
	}
	if got := Extract("我不喜欢私信内容"); len(got) != 0 {
		t.Fatalf("private message extracted: %+v", got)
	}
	store := NewMapStore()
	if err := store.Apply(context.Background(), 2, Candidate{
		Layer: LayerProfile, Dimension: "topic", Value: "13800138000", Score: 0.5,
		Source: SourceConversation, Confidence: 0.9,
	}, time.Now()); err == nil {
		t.Fatal("expected reject")
	}
}

func TestApplyIgnoresBehaviorWhenPersonalizationDisabled(t *testing.T) {
	store := NewMapStore()
	store.Personalization = func(context.Context, int64) (bool, error) { return false, nil }
	if err := store.Apply(context.Background(), 2, Candidate{
		Layer: LayerProfile, Dimension: "topic", Value: "behavior-topic", Score: 0.8,
		Source: SourceBehavior, Confidence: 0.6,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(context.Background(), 2, LayerProfile, time.Now())
	if err != nil || len(items) != 0 {
		t.Fatalf("behavior write must be ignored: %+v err=%v", items, err)
	}
	if err := store.Apply(context.Background(), 2, Candidate{
		Layer: LayerProfile, Dimension: "topic", Value: "explicit-topic", Score: 0.8,
		Source: SourceExplicit, Confidence: 1,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(context.Background(), 2, Candidate{
		Layer: LayerProfile, Dimension: "topic", Value: "behavior-visible", Score: 0.5,
		Source: SourceBehavior, Confidence: 0.5,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	store.Personalization = func(context.Context, int64) (bool, error) { return true, nil }
	if err := store.Apply(context.Background(), 2, Candidate{
		Layer: LayerProfile, Dimension: "topic", Value: "behavior-visible", Score: 0.5,
		Source: SourceBehavior, Confidence: 0.5,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	block, err := store.ContextBlock(context.Background(), 2, "recommend", time.Now(), true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(block, "behavior-visible") || !strings.Contains(block, "explicit-topic") {
		t.Fatalf("%q", block)
	}
}

func TestPackRoundTrip(t *testing.T) {
	id := PackID(LayerProfile, 9)
	layer, rec, ok := UnpackID(id)
	if !ok || layer != LayerProfile || rec != 9 {
		t.Fatalf("%s %d %v", layer, rec, ok)
	}
}
