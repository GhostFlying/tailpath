package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/GhostFlying/tailpath/exporter"
)

type runtimeSpec struct {
	key      string
	hostname string
	stateDir string
}

type runtimeInstance interface {
	exporter.Source
	Close() error
}

type runtimeFactory interface {
	Start(context.Context, runtimeSpec) (runtimeInstance, error)
}

type observerRegistration interface {
	Withdraw(context.Context) error
}

type observerRegistry interface {
	Register(string, exporter.Source) (observerRegistration, error)
}

type managedRuntime struct {
	spec         runtimeSpec
	instance     runtimeInstance
	registration observerRegistration
}

type runtimeManager struct {
	mu       sync.Mutex
	factory  runtimeFactory
	registry observerRegistry
	runtimes map[string]*managedRuntime
}

func newRuntimeManager(factory runtimeFactory, registry observerRegistry) *runtimeManager {
	return &runtimeManager{factory: factory, registry: registry, runtimes: make(map[string]*managedRuntime)}
}

func (m *runtimeManager) Add(ctx context.Context, spec runtimeSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addLocked(ctx, spec)
}

func (m *runtimeManager) addLocked(ctx context.Context, spec runtimeSpec) error {
	if _, exists := m.runtimes[spec.key]; exists {
		return fmt.Errorf("runtime %q is already registered", spec.key)
	}
	instance, err := m.factory.Start(ctx, spec)
	if err != nil {
		return fmt.Errorf("start runtime %q: %w", spec.key, err)
	}
	registration, err := m.registry.Register(spec.key, instance)
	if err != nil {
		return errors.Join(fmt.Errorf("register runtime %q: %w", spec.key, err), instance.Close())
	}
	m.runtimes[spec.key] = &managedRuntime{spec: spec, instance: instance, registration: registration}
	return nil
}

func (m *runtimeManager) Remove(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeLocked(ctx, key)
}

func (m *runtimeManager) removeLocked(ctx context.Context, key string) error {
	runtime := m.runtimes[key]
	if runtime == nil {
		return fmt.Errorf("runtime %q is not registered", key)
	}
	if err := runtime.registration.Withdraw(ctx); err != nil {
		return fmt.Errorf("withdraw runtime %q: %w", key, err)
	}
	delete(m.runtimes, key)
	if err := runtime.instance.Close(); err != nil {
		return fmt.Errorf("close runtime %q: %w", key, err)
	}
	return nil
}

func (m *runtimeManager) Restart(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.runtimes[key]
	if runtime == nil {
		return fmt.Errorf("runtime %q is not registered", key)
	}
	spec := runtime.spec
	if err := m.removeLocked(ctx, key); err != nil {
		return err
	}
	return m.addLocked(ctx, spec)
}

func (m *runtimeManager) Runtime(key string) (runtimeInstance, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.runtimes[key]
	if runtime == nil {
		return nil, false
	}
	return runtime.instance, true
}

func (m *runtimeManager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.runtimes))
	for key := range m.runtimes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result error
	for _, key := range keys {
		runtime := m.runtimes[key]
		withdrawErr := runtime.registration.Withdraw(ctx)
		closeErr := runtime.instance.Close()
		delete(m.runtimes, key)
		if withdrawErr != nil {
			result = errors.Join(result, fmt.Errorf("withdraw runtime %q: %w", key, withdrawErr))
		}
		if closeErr != nil {
			result = errors.Join(result, fmt.Errorf("close runtime %q: %w", key, closeErr))
		}
	}
	return result
}

type sinkRegistry struct {
	sink *exporter.SnapshotSink
}

func (r sinkRegistry) Register(key string, source exporter.Source) (observerRegistration, error) {
	return r.sink.Register(key, source)
}
