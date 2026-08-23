package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/tsnet"

	"github.com/GhostFlying/tailpath/internal/aggregate"
	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/collector"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/fixtures"
	"github.com/GhostFlying/tailpath/internal/httpapi"
	"github.com/GhostFlying/tailpath/internal/store"
	"github.com/GhostFlying/tailpath/internal/tailscaleadapter"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("tailpath stopped", "error", err)
		os.Exit(1)
	}
}

func run(arguments []string, logger *slog.Logger) error {
	if len(arguments) == 0 {
		usage()
		return flag.ErrHelp
	}
	switch arguments[0] {
	case "server":
		return runServer(arguments[1:], logger, false)
	case "fixture-server":
		return runServer(arguments[1:], logger, true)
	case "collector":
		return runCollector(arguments[1:], logger)
	case "healthcheck":
		return runHealthcheck(arguments[1:])
	case "version", "--version", "-version":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tailpath <server|collector|fixture-server|healthcheck|version> [flags]")
}

func runServer(arguments []string, logger *slog.Logger, fixture bool) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	networkMode := flags.String("network", "tsnet", "listener mode: tsnet or tailscaled")
	listenAddress := flags.String("listen", ":8080", "Tailnet HTTP listen address")
	hostname := flags.String("hostname", "tailpath", "tsnet hostname")
	stateDir := flags.String("state-dir", "tailpath-tsnet", "tsnet state directory")
	databasePath := flags.String("database", "tailpath.db", "SQLite database path")
	webDir := flags.String("web-dir", "web/dist", "built web application directory")
	socket := flags.String("socket", "", "tailscaled LocalAPI socket")
	adminListen := flags.String("admin-listen", "127.0.0.1:8091", "loopback health listen address")
	heartbeat := flags.Duration("heartbeat-interval", time.Minute, "observer freshness heartbeat interval")
	unsafeBroadListen := flags.Bool("unsafe-allow-non-tailnet-listen", false, "allow tailscaled mode to bind a non-Tailscale address; API WhoIs remains required")
	scaleFixture := flags.Bool("scale", false, "load the 250-node/1,000-edge test fixture")
	if fixture {
		*networkMode = "plain"
		*databasePath = ":memory:"
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *scaleFixture && !fixture {
		return errors.New("scale fixture is only available with fixture-server")
	}
	if *scaleFixture && *heartbeat == time.Minute {
		*heartbeat = 10 * time.Minute
	}
	if *heartbeat <= 0 {
		return errors.New("heartbeat interval must be positive")
	}
	if err := ensureDatabaseDirectory(*databasePath); err != nil {
		return err
	}
	database, err := store.Open(*databasePath, 7*24*time.Hour)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var listener net.Listener
	var authorizer httpapi.Authorizer
	var controlIDs []string

	switch *networkMode {
	case "plain":
		if !fixture {
			return errors.New("plain networking is reserved for fixture-server")
		}
		listener, err = net.Listen("tcp", *listenAddress)
		authorizer = httpapi.LoopbackAuthorizer{}
	case "tailscaled":
		client := &local.Client{Socket: *socket, UseSocketOnly: *socket != ""}
		controlIDs, err = tailscaleadapter.ControlStableNodeIDs(ctx, client)
		if err != nil {
			return fmt.Errorf("read tailscaled identity: %w", err)
		}
		*listenAddress, err = tailscaledListenAddress(ctx, client, *listenAddress, *unsafeBroadListen)
		if err != nil {
			return err
		}
		listener, err = net.Listen("tcp", *listenAddress)
		authorizer = tailscaleadapter.NewAuthorizer(client)
	case "tsnet":
		authKey, authKeyErr := resolveAuthKey(os.Getenv("TS_AUTHKEY"))
		if authKeyErr != nil {
			return fmt.Errorf("read tsnet auth key: %w", authKeyErr)
		}
		server := &tsnet.Server{
			Dir:      *stateDir,
			Hostname: *hostname,
			AuthKey:  authKey,
			Logf: func(format string, values ...any) {
				logger.Debug(fmt.Sprintf(format, values...))
			},
		}
		if err := server.Start(); err != nil {
			return fmt.Errorf("start tsnet server: %w", err)
		}
		defer server.Close()
		if _, err := server.Up(ctx); err != nil {
			return fmt.Errorf("bring tsnet server up: %w", err)
		}
		listener, err = server.Listen("tcp", *listenAddress)
		if err != nil {
			break
		}
		client, clientErr := server.LocalClient()
		if clientErr != nil {
			return clientErr
		}
		controlIDs, err = tailscaleadapter.ControlStableNodeIDs(ctx, client)
		authorizer = tailscaleadapter.NewAuthorizer(client)
	default:
		return fmt.Errorf("unknown network mode %q", *networkMode)
	}
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	application, err := app.New(database, aggregate.Options{
		HeartbeatInterval: *heartbeat,
		ControlNodeIDs:    controlIDs,
	}, logger)
	if err != nil {
		return err
	}
	go application.RunMaintenance(ctx)
	server := httpapi.New(application, httpapi.Options{Authorizer: authorizer, WebDir: *webDir, Logger: logger})
	if fixture {
		if *scaleFixture {
			scenario, err := fixtures.NewScaleScenario(fixtures.DefaultScaleConfig())
			if err != nil {
				return err
			}
			if err := scenario.Load(ctx, application, time.Now().UTC()); err != nil {
				return fmt.Errorf("load scale fixture: %w", err)
			}
			if err := scenario.RefreshRuntime(application.Aggregator, time.Now().UTC(), 4); err != nil {
				return fmt.Errorf("refresh scale fixture: %w", err)
			}
			go runScaleRuntime(ctx, scenario, application.Aggregator, logger)
		} else {
			go fixtures.New(application, logger).Run(ctx)
		}
	}
	logger.Info("server listening", "network", *networkMode, "address", listener.Addr())
	return serve(ctx, listener, server.Handler(), *adminListen, logger)
}

func runScaleRuntime(ctx context.Context, scenario *fixtures.ScaleScenario, aggregator *aggregate.Aggregator, logger *slog.Logger) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	sequence := int64(5)
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			if err := scenario.RefreshRuntime(aggregator, at.UTC(), sequence); err != nil {
				logger.Error("scale fixture refresh failed", "error", err)
				return
			}
			sequence++
		}
	}
}

func resolveAuthKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "file:") {
		return value, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(value, "file:"))
	if path == "" {
		return "", errors.New("auth key file path is empty")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(contents)), nil
}

func tailscaledListenAddress(ctx context.Context, client *local.Client, requested string, unsafeBroadListen bool) (string, error) {
	host, port, err := net.SplitHostPort(requested)
	if err != nil {
		return "", fmt.Errorf("parse listen address: %w", err)
	}
	status, err := client.StatusWithoutPeers(ctx)
	if err != nil {
		return "", fmt.Errorf("read local Tailscale IP: %w", err)
	}
	if len(status.TailscaleIPs) == 0 {
		return "", errors.New("tailscaled has no assigned Tailscale IP")
	}
	addresses := make([]string, 0, len(status.TailscaleIPs))
	for _, address := range status.TailscaleIPs {
		addresses = append(addresses, address.String())
	}
	return validateTailscaledListenAddress(host, port, addresses, unsafeBroadListen)
}

func validateTailscaledListenAddress(host, port string, tailscaleIPs []string, unsafeBroadListen bool) (string, error) {
	if host == "" {
		return net.JoinHostPort(tailscaleIPs[0], port), nil
	}
	for _, address := range tailscaleIPs {
		if host == address {
			return net.JoinHostPort(host, port), nil
		}
	}
	if unsafeBroadListen {
		return net.JoinHostPort(host, port), nil
	}
	return "", fmt.Errorf("tailscaled listener %q is not a local Tailscale IP; use --unsafe-allow-non-tailnet-listen to override", host)
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler, adminAddress string, logger *slog.Logger) error {
	baseContext := func(net.Listener) context.Context { return ctx }
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 75 * time.Second,
		BaseContext: baseContext,
	}
	adminListener, err := net.Listen("tcp", adminAddress)
	if err != nil {
		return fmt.Errorf("listen on admin address: %w", err)
	}
	adminServer := &http.Server{
		Handler: httpapi.HealthHandler(), ReadHeaderTimeout: 5 * time.Second,
		BaseContext: baseContext,
	}
	errorsChannel := make(chan error, 2)
	go func() { errorsChannel <- server.Serve(listener) }()
	go func() { errorsChannel <- adminServer.Serve(adminListener) }()
	select {
	case <-ctx.Done():
	case err := <-errorsChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Warn("application server shutdown failed", "error", err)
	}
	if err := adminServer.Shutdown(shutdownContext); err != nil {
		logger.Warn("admin server shutdown failed", "error", err)
	}
	return nil
}

func runCollector(arguments []string, logger *slog.Logger) error {
	config, err := parseCollectorConfig(arguments, os.Getenv)
	if err != nil {
		return err
	}
	source := tailscaleadapter.NewLocalSource(config.socket)
	if config.check {
		return checkCollector(context.Background(), source, os.Stdout)
	}
	reporter, err := collector.NewHTTPReporter(config.serverURL, nil)
	if err != nil {
		return err
	}
	runner := collector.New(source, reporter, collector.Options{
		Logger: logger,
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("collector started", "server", config.serverURL, "sample_interval", 2*time.Second)
	return runner.Run(ctx)
}

type collectorConfig struct {
	serverURL string
	socket    string
	check     bool
}

func parseCollectorConfig(arguments []string, getenv func(string) string) (collectorConfig, error) {
	flags := flag.NewFlagSet("collector", flag.ContinueOnError)
	serverDefault := getenv("TAILPATH_SERVER_URL")
	if serverDefault == "" {
		serverDefault = "http://tailpath:8080"
	}
	serverURL := flags.String("server", serverDefault, "Tailpath server URL on the Tailnet")
	socket := flags.String("socket", getenv("TAILPATH_SOCKET"), "tailscaled LocalAPI socket")
	check := flags.Bool("check", false, "read LocalAPI status once without reporting")
	if err := flags.Parse(arguments); err != nil {
		return collectorConfig{}, err
	}
	if flags.NArg() != 0 {
		return collectorConfig{}, fmt.Errorf("collector does not accept positional arguments")
	}
	return collectorConfig{serverURL: *serverURL, socket: *socket, check: *check}, nil
}

type collectorCheckResult struct {
	Self      domain.NodeIdentity `json:"self"`
	OS        string              `json:"os"`
	PeerCount int                 `json:"peerCount"`
}

func checkCollector(ctx context.Context, source collector.Source, output io.Writer) error {
	snapshot, err := source.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("read local status: %w", err)
	}
	result := collectorCheckResult{
		Self:      snapshot.Observer,
		OS:        collectorRuntimeOS(runtime.GOOS),
		PeerCount: len(snapshot.Peers),
	}
	if err := json.NewEncoder(output).Encode(result); err != nil {
		return fmt.Errorf("write collector check: %w", err)
	}
	return nil
}

func collectorRuntimeOS(value string) string {
	if value == "darwin" {
		return "macos"
	}
	return value
}

func runHealthcheck(arguments []string) error {
	flags := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	target := flags.String("url", "http://127.0.0.1:8091/healthz", "health endpoint")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	parsed, err := url.Parse(*target)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return errors.New("healthcheck URL must be an http URL with a host")
	}
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Get(*target)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned %s", response.Status)
	}
	return nil
}

func ensureDatabaseDirectory(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	directory := filepath.Dir(path)
	if directory == "." {
		return nil
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}
