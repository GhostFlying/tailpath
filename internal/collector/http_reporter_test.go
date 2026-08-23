package collector

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

func TestHTTPReporterDefaultClientBypassesProxy(t *testing.T) {
	reporter, err := NewHTTPReporter("http://100.64.0.1:8080", nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := reporter.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default transport = %T, want *http.Transport", reporter.client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("default reporter transport must connect directly over the Tailnet")
	}
}

func TestHTTPReporterReturnsTypedStatusErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(status)
				_, _ = response.Write([]byte("diagnostic body"))
			}))
			defer server.Close()
			reporter, err := NewHTTPReporter(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = reporter.Send(context.Background(), domain.ReportEnvelope{})
			var statusError *HTTPStatusError
			if !errors.As(err, &statusError) {
				t.Fatalf("Send error = %T %v, want HTTPStatusError", err, err)
			}
			if statusError.StatusCode != status || statusError.Message != "diagnostic body" {
				t.Fatalf("status error = %#v", statusError)
			}
		})
	}
}

func TestHTTPReporterHonorsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		response.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	reporter, err := NewHTTPReporter(server.URL, &http.Client{Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reporter.Send(context.Background(), domain.ReportEnvelope{})
	if err == nil {
		t.Fatal("Send unexpectedly completed before timeout")
	}
}

func TestHTTPReporterPreservesCustomClient(t *testing.T) {
	client := &http.Client{}
	reporter, err := NewHTTPReporter("https://tailpath.example", client)
	if err != nil {
		t.Fatal(err)
	}
	if reporter.client != client {
		t.Fatal("custom client was replaced")
	}
}
