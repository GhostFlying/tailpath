package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPReporter struct {
	endpoint        string
	capabilitiesURL string
	client          *http.Client
}

type IncompatibleServerError struct {
	Reason string
}

func (e *IncompatibleServerError) Error() string {
	return "incompatible Tailpath server: " + e.Reason
}

type HTTPStatusError struct {
	StatusCode int
	Status     string
	Message    string
}

func (e *HTTPStatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("server returned %s", e.Status)
	}
	return fmt.Sprintf("server returned %s: %s", e.Status, e.Message)
}

func NewHTTPReporter(serverURL string, client *http.Client) (*HTTPReporter, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("server URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("server URL requires a host")
	}
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		client = &http.Client{Transport: transport, Timeout: 15 * time.Second}
	}
	return &HTTPReporter{
		endpoint:        strings.TrimSuffix(serverURL, "/") + "/api/v1/reports",
		capabilitiesURL: strings.TrimSuffix(serverURL, "/") + "/api/v1/capabilities",
		client:          client,
	}, nil
}

func (r *HTTPReporter) Capabilities(ctx context.Context) (Capabilities, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.capabilitiesURL, nil)
	if err != nil {
		return Capabilities{}, err
	}
	response, err := r.client.Do(request)
	if err != nil {
		return Capabilities{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Capabilities{}, &IncompatibleServerError{Reason: "capability endpoint is unavailable"}
	}
	if response.StatusCode != http.StatusOK {
		return Capabilities{}, responseStatusError(response)
	}
	var capabilities Capabilities
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&capabilities); err != nil {
		return Capabilities{}, &IncompatibleServerError{Reason: "invalid capability response: " + err.Error()}
	}
	if err := ensureEOF(decoder); err != nil {
		return Capabilities{}, &IncompatibleServerError{Reason: "invalid capability response: " + err.Error()}
	}
	return capabilities, nil
}

func (r *HTTPReporter) RequireCapabilities(ctx context.Context, features ...string) error {
	capabilities, err := r.Capabilities(ctx)
	if err != nil {
		return err
	}
	if !capabilities.SupportsProtocol(ProtocolVersion) {
		return &IncompatibleServerError{Reason: fmt.Sprintf("observer protocol %d is not supported", ProtocolVersion)}
	}
	for _, feature := range features {
		if !capabilities.SupportsFeature(feature) {
			return &IncompatibleServerError{Reason: fmt.Sprintf("required feature %q is unavailable", feature)}
		}
	}
	return nil
}

func (r *HTTPReporter) Send(ctx context.Context, report ReportEnvelope) (ReportReceipt, error) {
	body, err := json.Marshal(report)
	if err != nil {
		return ReportReceipt{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return ReportReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return ReportReceipt{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return ReportReceipt{}, responseStatusError(response)
	}
	var receipt ReportReceipt
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return ReportReceipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return ReportReceipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	return receipt, nil
}

func responseStatusError(response *http.Response) error {
	message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return &HTTPStatusError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Message:    strings.TrimSpace(string(message)),
	}
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("response contains more than one JSON value")
}

var _ Reporter = (*HTTPReporter)(nil)
