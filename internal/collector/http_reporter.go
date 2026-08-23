package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
)

type HTTPReporter struct {
	endpoint string
	client   *http.Client
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
		endpoint: strings.TrimSuffix(serverURL, "/") + "/api/v1/reports",
		client:   client,
	}, nil
}

func (r *HTTPReporter) Send(ctx context.Context, report domain.ReportEnvelope) (domain.ReportReceipt, error) {
	body, err := json.Marshal(report)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return domain.ReportReceipt{}, fmt.Errorf("server returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	var receipt domain.ReportReceipt
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("decode receipt: %w", err)
	}
	return receipt, nil
}
