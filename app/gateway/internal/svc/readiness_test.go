package svc

import (
	"testing"

	"google.golang.org/grpc/connectivity"
)

func stateProvider(state connectivity.State) func() connectivity.State {
	return func() connectivity.State { return state }
}

func TestReadinessAllRequiredUp(t *testing.T) {
	ctx := &ServiceContext{Dependencies: []Dependency{
		{Name: "user", ConnState: stateProvider(connectivity.Ready)},
		{Name: "search", ConnState: stateProvider(connectivity.Idle), Optional: true},
	}}
	status, dependencies := ctx.Readiness()
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
		{Name: "user", ConnState: stateProvider(connectivity.TransientFailure)},
		{Name: "search", ConnState: stateProvider(connectivity.Ready), Optional: true},
	}}
	status, _ := ctx.Readiness()
	if status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", status)
	}
}

func TestReadinessOptionalDownIsDegraded(t *testing.T) {
	ctx := &ServiceContext{Dependencies: []Dependency{
		{Name: "user", ConnState: stateProvider(connectivity.Ready)},
		{Name: "search", ConnState: stateProvider(connectivity.TransientFailure), Optional: true},
	}}
	status, dependencies := ctx.Readiness()
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
	status, _ := ctx.Readiness()
	if status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", status)
	}
}
