package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GhostFlying/tailpath/exporter"
)

type fakeRuntime struct {
	key    string
	events *[]string
}

func (r *fakeRuntime) Snapshot(context.Context) (exporter.Snapshot, error) {
	return exporter.Snapshot{Observer: exporter.NodeIdentity{StableNodeID: r.key}}, nil
}

func (r *fakeRuntime) Close() error {
	*r.events = append(*r.events, "close:"+r.key)
	return nil
}

type fakeFactory struct {
	events *[]string
	specs  []runtimeSpec
	err    error
}

func (f *fakeFactory) Start(_ context.Context, spec runtimeSpec) (runtimeInstance, error) {
	*f.events = append(*f.events, "start:"+spec.key)
	f.specs = append(f.specs, spec)
	if f.err != nil {
		return nil, f.err
	}
	return &fakeRuntime{key: spec.key, events: f.events}, nil
}

type fakeRegistration struct {
	key    string
	events *[]string
	err    error
}

func (r *fakeRegistration) Withdraw(context.Context) error {
	*r.events = append(*r.events, "withdraw:"+r.key)
	return r.err
}

type fakeRegistry struct {
	events      *[]string
	registerErr error
	withdrawErr error
}

func (r *fakeRegistry) Register(key string, _ exporter.Source) (observerRegistration, error) {
	*r.events = append(*r.events, "register:"+key)
	if r.registerErr != nil {
		return nil, r.registerErr
	}
	return &fakeRegistration{key: key, events: r.events, err: r.withdrawErr}, nil
}

func TestRuntimeManagerOrdersRestartAndShutdown(t *testing.T) {
	events := []string{}
	factory := &fakeFactory{events: &events}
	manager := newRuntimeManager(factory, &fakeRegistry{events: &events})
	spec := runtimeSpec{key: "runtime-c", hostname: "example-runtime-c", stateDir: "/state/runtime-c"}
	if err := manager.Add(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if err := manager.Restart(context.Background(), spec.key); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"start:runtime-c", "register:runtime-c",
		"withdraw:runtime-c", "close:runtime-c", "start:runtime-c", "register:runtime-c",
		"withdraw:runtime-c", "close:runtime-c",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %#v, want %#v", events, want)
	}
	if len(factory.specs) != 2 || factory.specs[0].stateDir != factory.specs[1].stateDir {
		t.Fatalf("restart specs = %#v", factory.specs)
	}
}

func TestRuntimeManagerClosesUnregisteredInstance(t *testing.T) {
	events := []string{}
	manager := newRuntimeManager(
		&fakeFactory{events: &events},
		&fakeRegistry{events: &events, registerErr: errors.New("registration failed")},
	)
	err := manager.Add(context.Background(), runtimeSpec{key: "runtime-a"})
	if err == nil || !reflect.DeepEqual(events, []string{"start:runtime-a", "register:runtime-a", "close:runtime-a"}) {
		t.Fatalf("add rollback events = %#v, err=%v", events, err)
	}
}

func TestRuntimeManagerPreservesRuntimeAfterFailedWithdrawal(t *testing.T) {
	events := []string{}
	registry := &fakeRegistry{events: &events, withdrawErr: errors.New("server unavailable")}
	manager := newRuntimeManager(&fakeFactory{events: &events}, registry)
	if err := manager.Add(context.Background(), runtimeSpec{key: "runtime-a"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), "runtime-a"); err == nil {
		t.Fatal("failed withdrawal was accepted")
	}
	if _, exists := manager.runtimes["runtime-a"]; !exists {
		t.Fatal("failed withdrawal removed runtime ownership")
	}
	_ = manager.Close(context.Background())
	if events[len(events)-1] != "close:runtime-a" {
		t.Fatalf("shutdown did not close retained runtime: %#v", events)
	}
}

func TestRuntimeSpecsUseIndependentPersistedIdentities(t *testing.T) {
	config := config{stateDir: "/state", hostnamePrefix: "example"}
	specs := runtimeSpecs(config)
	if len(specs) != 3 {
		t.Fatalf("runtime specs = %#v", specs)
	}
	seen := map[string]struct{}{}
	for _, spec := range specs {
		for _, value := range []string{spec.key, spec.hostname, spec.stateDir} {
			if _, duplicate := seen[value]; duplicate {
				t.Fatalf("duplicate runtime attribute %q in %#v", value, specs)
			}
			seen[value] = struct{}{}
		}
	}
}
