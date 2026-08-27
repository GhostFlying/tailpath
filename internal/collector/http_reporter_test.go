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

func TestHTTPReporterCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/capabilities" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"observerProtocolVersions":[1],"features":["multi-observer"]}`))
	}))
	defer server.Close()
	reporter, err := NewHTTPReporter(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.RequireCapabilities(context.Background(), domain.FeatureMultiObserver); err != nil {
		t.Fatal(err)
	}
	err = reporter.RequireCapabilities(context.Background(), domain.FeatureObserverWithdrawal)
	var incompatible *IncompatibleServerError
	if !errors.As(err, &incompatible) {
		t.Fatalf("missing feature error = %T %v", err, err)
	}
}

func TestHTTPReporterCapabilitiesClassifyFailures(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		body         string
		incompatible bool
	}{
		{name: "old server", status: http.StatusNotFound, incompatible: true},
		{name: "malformed", status: http.StatusOK, body: `{`, incompatible: true},
		{name: "unauthorized", status: http.StatusUnauthorized, body: "denied"},
		{name: "server failure", status: http.StatusInternalServerError, body: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			reporter, err := NewHTTPReporter(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = reporter.Capabilities(context.Background())
			var incompatible *IncompatibleServerError
			var statusError *HTTPStatusError
			if test.incompatible && !errors.As(err, &incompatible) {
				t.Fatalf("error = %T %v, want IncompatibleServerError", err, err)
			}
			if !test.incompatible && !errors.As(err, &statusError) {
				t.Fatalf("error = %T %v, want HTTPStatusError", err, err)
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
