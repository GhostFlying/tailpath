package collector

import (
	"net/http"
	"testing"
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
