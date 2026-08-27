package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tailscaletsnet "tailscale.com/tsnet"

	"github.com/GhostFlying/tailpath/exporter"
	tailpathtsnet "github.com/GhostFlying/tailpath/exporter/tsnet"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], os.Getenv, logger); err != nil {
		logger.Error("tsnet exporter example stopped", "error", err)
		os.Exit(1)
	}
}

func run(arguments []string, getenv func(string) string, logger *slog.Logger) error {
	config, err := parseConfig(arguments, getenv)
	if err != nil {
		return err
	}
	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	reporterServer := newTSNetServer(runtimeSpec{
		key: "reporter", hostname: config.hostnamePrefix + "-reporter",
		stateDir: filepath.Join(config.stateDir, "reporter"),
	}, config, logger)
	if err := bringUp(signalContext, reporterServer); err != nil {
		return fmt.Errorf("start reporting identity: %w", err)
	}
	defer reporterServer.Close()
	reporter, err := exporter.NewHTTPReporter(config.serverURL,
		clientWithTimeout(reporterServer.HTTPClient(), 15*time.Second))
	if err != nil {
		return err
	}
	sink := exporter.NewSnapshotSink(reporter, exporter.SnapshotSinkOptions{Logger: logger})
	manager := newRuntimeManager(tsnetRuntimeFactory{config: config, logger: logger}, sinkRegistry{sink: sink})
	specs := runtimeSpecs(config)
	for _, spec := range specs {
		if err := manager.Add(signalContext, spec); err != nil {
			_ = manager.Close(context.Background())
			return err
		}
	}

	sinkContext, stopSink := context.WithCancel(context.Background())
	sinkDone := make(chan error, 1)
	go func() { sinkDone <- sink.Run(sinkContext) }()
	var workload *dogfoodWorkload
	if config.workloadDemo {
		workload, err = startDogfoodWorkload(signalContext, manager, logger)
		if err != nil {
			stopSink()
			<-sinkDone
			_ = manager.Close(context.Background())
			return err
		}
	}
	if config.lifecycleDemo {
		go runLifecycleDemo(signalContext, manager, specs[2], config.lifecycleStep, logger)
	}
	logger.Info("tsnet exporter example started", "runtimes", len(specs),
		"lifecycle_demo", config.lifecycleDemo, "workload_demo", config.workloadDemo)

	var runErr error
	sinkExited := false
	select {
	case <-signalContext.Done():
	case runErr = <-sinkDone:
		sinkExited = true
		stopSignals()
	case runErr = <-workloadFailure(workload):
		stopSignals()
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if workload != nil {
		runErr = errors.Join(runErr, workload.Close(shutdownContext))
	}
	closeErr := manager.Close(shutdownContext)
	stopSink()
	if !sinkExited {
		select {
		case sinkErr := <-sinkDone:
			if runErr == nil {
				runErr = sinkErr
			}
		case <-shutdownContext.Done():
			runErr = errors.Join(runErr, errors.New("snapshot sink shutdown timed out"))
		}
	}
	return errors.Join(runErr, closeErr)
}

func clientWithTimeout(client *http.Client, timeout time.Duration) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	bounded := *client
	bounded.Timeout = timeout
	return &bounded
}

type tsnetRuntimeFactory struct {
	config config
	logger *slog.Logger
}

func (f tsnetRuntimeFactory) Start(ctx context.Context, spec runtimeSpec) (runtimeInstance, error) {
	server := newTSNetServer(spec, f.config, f.logger)
	if err := bringUp(ctx, server); err != nil {
		_ = server.Close()
		return nil, err
	}
	source, err := tailpathtsnet.New(server)
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	return &tsnetRuntime{Source: source, server: server}, nil
}

type tsnetRuntime struct {
	*tailpathtsnet.Source
	server *tailscaletsnet.Server
}

func (r *tsnetRuntime) Listen(network, address string) (net.Listener, error) {
	return r.server.Listen(network, address)
}

func (r *tsnetRuntime) HTTPClient() *http.Client {
	return r.server.HTTPClient()
}

func (r *tsnetRuntime) TailscaleIPv4() string {
	ipv4, _ := r.server.TailscaleIPs()
	return ipv4.String()
}

func (r *tsnetRuntime) Close() error {
	return r.server.Close()
}

func newTSNetServer(spec runtimeSpec, config config, logger *slog.Logger) *tailscaletsnet.Server {
	return &tailscaletsnet.Server{
		Dir: spec.stateDir, Hostname: spec.hostname, AuthKey: config.authKey, ControlURL: config.controlURL,
		Logf: func(format string, values ...any) { logger.Debug(fmt.Sprintf(format, values...)) },
	}
}

func bringUp(ctx context.Context, server *tailscaletsnet.Server) error {
	if err := server.Start(); err != nil {
		return err
	}
	_, err := server.Up(ctx)
	return err
}

func runtimeSpecs(config config) []runtimeSpec {
	result := make([]runtimeSpec, 3)
	for index, suffix := range []string{"a", "b", "c"} {
		key := "runtime-" + suffix
		result[index] = runtimeSpec{
			key: key, hostname: config.hostnamePrefix + "-" + key,
			stateDir: filepath.Join(config.stateDir, key),
		}
	}
	return result
}

func runLifecycleDemo(
	ctx context.Context,
	manager *runtimeManager,
	spec runtimeSpec,
	step time.Duration,
	logger *slog.Logger,
) {
	if !waitFor(ctx, step) {
		return
	}
	if err := manager.Remove(ctx, spec.key); err != nil {
		logger.Warn("lifecycle demo withdrawal failed", "runtime", spec.key, "error", err)
		return
	}
	logger.Info("lifecycle demo runtime withdrawn", "runtime", spec.key)
	if !waitFor(ctx, step) {
		return
	}
	if err := manager.Add(ctx, spec); err != nil {
		logger.Warn("lifecycle demo restart failed", "runtime", spec.key, "error", err)
		return
	}
	logger.Info("lifecycle demo runtime restarted", "runtime", spec.key)
}

func waitFor(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
