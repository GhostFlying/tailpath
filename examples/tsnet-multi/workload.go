package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	dogfoodWorkloadBytes    = 8 << 20
	dogfoodWorkloadInterval = 2 * time.Second
)

type workloadRuntime interface {
	Listen(string, string) (net.Listener, error)
	HTTPClient() *http.Client
	TailscaleIPv4() string
}

type dogfoodWorkload struct {
	cancel     context.CancelFunc
	server     *http.Server
	listener   net.Listener
	clientDone chan struct{}
	failure    chan error
	completed  atomic.Int64
	closeOnce  sync.Once
	closeErr   error
}

func startDogfoodWorkload(
	parent context.Context,
	manager *runtimeManager,
	logger *slog.Logger,
) (*dogfoodWorkload, error) {
	sourceValue, sourceExists := manager.Runtime("runtime-a")
	targetValue, targetExists := manager.Runtime("runtime-b")
	source, sourceOK := sourceValue.(workloadRuntime)
	target, targetOK := targetValue.(workloadRuntime)
	if !sourceExists || !targetExists || !sourceOK || !targetOK {
		return nil, errors.New("dogfood workload requires runtime-a and runtime-b Tailnet transports")
	}
	listener, err := target.Listen("tcp", ":18088")
	if err != nil {
		return nil, errors.New("start dogfood workload listener")
	}
	port, err := listenerPort(listener.Addr())
	if err != nil {
		listener.Close()
		return nil, err
	}
	targetIP := target.TailscaleIPv4()
	if net.ParseIP(targetIP) == nil {
		listener.Close()
		return nil, errors.New("dogfood workload target has no IPv4 address")
	}
	baseClient := source.HTTPClient()
	if baseClient == nil {
		listener.Close()
		return nil, errors.New("dogfood workload source has no HTTP client")
	}
	client := *baseClient
	client.Timeout = 30 * time.Second

	ctx, cancel := context.WithCancel(parent)
	workload := &dogfoodWorkload{
		cancel: cancel, listener: listener, clientDone: make(chan struct{}),
		failure: make(chan error, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("Content-Type", "application/octet-stream")
		response.Header().Set("Content-Length", fmt.Sprint(dogfoodWorkloadBytes))
		_, _ = io.CopyN(response, zeroReader{}, dogfoodWorkloadBytes)
	})
	workload.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if serveErr := workload.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case workload.failure <- errors.New("dogfood workload listener stopped"):
			default:
			}
		}
	}()
	targetURL := "http://" + net.JoinHostPort(targetIP, port) + "/stream"
	go workload.runClient(ctx, &client, targetURL, logger)
	logger.Info("dogfood workload started", "bytes_per_transfer", dogfoodWorkloadBytes)
	return workload, nil
}

func (w *dogfoodWorkload) runClient(ctx context.Context, client *http.Client, targetURL string, logger *slog.Logger) {
	defer close(w.clientDone)
	degraded := false
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err == nil {
			request.Close = true
			var response *http.Response
			response, err = client.Do(request)
			if err == nil {
				read, copyErr := io.Copy(io.Discard, response.Body)
				closeErr := response.Body.Close()
				if response.StatusCode != http.StatusOK || read != dogfoodWorkloadBytes || copyErr != nil || closeErr != nil {
					err = errors.New("incomplete dogfood workload response")
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !degraded {
				logger.Warn("dogfood workload degraded")
				degraded = true
			}
		} else {
			w.completed.Add(1)
			if degraded {
				logger.Info("dogfood workload recovered")
				degraded = false
			}
		}
		timer := time.NewTimer(dogfoodWorkloadInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (w *dogfoodWorkload) Close(ctx context.Context) error {
	w.closeOnce.Do(func() {
		w.cancel()
		shutdownErr := w.server.Shutdown(ctx)
		select {
		case <-w.clientDone:
		case <-ctx.Done():
			shutdownErr = errors.Join(shutdownErr, ctx.Err())
		}
		w.closeErr = shutdownErr
	})
	return w.closeErr
}

func workloadFailure(workload *dogfoodWorkload) <-chan error {
	if workload == nil {
		return nil
	}
	return workload.failure
}

func listenerPort(address net.Addr) (string, error) {
	_, port, err := net.SplitHostPort(address.String())
	if err != nil || port == "" {
		return "", errors.New("dogfood workload listener has no TCP port")
	}
	return port, nil
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}
