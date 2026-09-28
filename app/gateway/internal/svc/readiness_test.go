package svc

import (
	"testing"

	"context"
	"errors"
)

func probe(healthy bool) func(context.Context) error {
	return func(context.Context) error {
		if healthy {
			return nil
		}
		return errors.New("dependency down")
	}
}

func TestReadinessAllRequiredUp(t *testing.T) {
	ctx := &ServiceContext{Dependencies: []Dependency{
		{Name: "user", Probe: probe(true)},
		{Name: "search", Probe: probe(true), Optional: true},
	}}
	status, dependencies := ctx.Readiness(context.Background())
	if status != "ready" {
		t.Fatalf("status = %q, want ready", status)
	}
	for _, dependency := range dependencies {
		if dependency.Status != "ok" {
			t.Fatalf("dependency %s status = %q, want ok", dependency.Name, dependency.Status)
		}
	}
}

func TestReadinessRequiredDownIsUnavailable(t *testing.T) {
	ctx := &ServiceContext{Dependencies: []Dependency{
		{Name: "user", Probe: probe(false)},
		{Name: "search", Probe: probe(true), Optional: true},
	}}
	status, _ := ctx.Readiness(context.Background())
	if status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", status)
	}
}

func TestReadinessOptionalDownIsDegraded(t *testing.T) {
	ctx := &ServiceContext{Dependencies: []Dependency{
		{Name: "user", Probe: probe(true)},
		{Name: "search", Probe: probe(false), Optional: true},
	}}
	status, dependencies := ctx.Readiness(context.Background())
	if status != "degraded" {
		t.Fatalf("status = %q, want degraded", status)
	}
	for _, dependency := range dependencies {
		if dependency.Name == "search" && dependency.Status != "down" {
			t.Fatalf("optional dependency should be marked down, got %q", dependency.Status)
		}
	}
}

func TestReadinessNilStateProviderIsDown(t *testing.T) {
	ctx := &ServiceContext{Dependencies: []Dependency{{Name: "user"}}}
	status, _ := ctx.Readiness(context.Background())
	if status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", status)
	}
}
