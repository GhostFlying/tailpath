package perfgate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/fixtures"
)

const ResultVersion = 1

type Config struct {
	ServerURL         string
	Duration          time.Duration
	ReportsPerSecond  int
	Workers           int
	RequestTimeout    time.Duration
	APISamples        int
	IngestP95Limit    time.Duration
	IngestP99Limit    time.Duration
	TopologyP95Limit  time.Duration
	HistoryP95Limit   time.Duration
	SchedulerLagLimit time.Duration
}

type Result struct {
	Version              int            `json:"version"`
	StartedAt            time.Time      `json:"startedAt"`
	FinishedAt           time.Time      `json:"finishedAt"`
	TargetDurationMS     int64          `json:"targetDurationMs"`
	TargetReportsPerSec  int            `json:"targetReportsPerSecond"`
	WarmupHellos         int            `json:"warmupHellos"`
	ScheduledReports     int            `json:"scheduledReports"`
	AcceptedReports      int            `json:"acceptedReports"`
	RejectedReceipts     int            `json:"rejectedReceipts"`
	HTTPFailures         int            `json:"httpFailures"`
	HTTP500s             int            `json:"http500s"`
	RequestErrors        int            `json:"requestErrors"`
	SchedulerMaxLagMS    float64        `json:"schedulerMaxLagMs"`
	CompletionDurationMS int64          `json:"completionDurationMs"`
	CompletionRPS        float64        `json:"completionReportsPerSecond"`
	Ingest               LatencySummary `json:"ingest"`
	Topology             LatencySummary `json:"topology"`
	HistoryList          LatencySummary `json:"historyList"`
	HistoryDetail        LatencySummary `json:"historyDetail"`
	TopologyNodes        int            `json:"topologyNodes"`
	TopologyEdges        int            `json:"topologyEdges"`
	Passed               bool           `json:"passed"`
	Failures             []string       `json:"failures,omitempty"`
}

type LatencySummary struct {
	Samples int     `json:"samples"`
	P50MS   float64 `json:"p50Ms"`
	P95MS   float64 `json:"p95Ms"`
	P99MS   float64 `json:"p99Ms"`
	MaxMS   float64 `json:"maxMs"`
}

type scheduledReport struct {
	report domain.ReportEnvelope
	due    time.Time
}

type requestResult struct {
	latency      time.Duration
	schedulerLag time.Duration
	status       int
	receipt      domain.ReportReceipt
	err          error
}

func DefaultConfig() Config {
	return Config{
		ServerURL:         "http://127.0.0.1:18082",
		Duration:          10 * time.Minute,
		ReportsPerSecond:  125,
		Workers:           32,
		RequestTimeout:    10 * time.Second,
		APISamples:        50,
		IngestP95Limit:    100 * time.Millisecond,
		IngestP99Limit:    250 * time.Millisecond,
		TopologyP95Limit:  250 * time.Millisecond,
		HistoryP95Limit:   500 * time.Millisecond,
		SchedulerLagLimit: time.Second,
	}
}

func Run(ctx context.Context, config Config) (Result, error) {
	result := Result{
		Version:             ResultVersion,
		TargetDurationMS:    config.Duration.Milliseconds(),
		TargetReportsPerSec: config.ReportsPerSecond,
	}
	if err := validateConfig(config); err != nil {
		return result, err
	}

	scenario, err := fixtures.NewScaleScenario(fixtures.DefaultScaleConfig())
	if err != nil {
		return result, err
	}
	client := &http.Client{
		Timeout: config.RequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        config.Workers * 2,
			MaxIdleConnsPerHost: config.Workers,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	defer client.CloseIdleConnections()

	warmupAt := time.Now().UTC()
	for _, report := range scenario.HelloReports(warmupAt) {
		requestResult := submitReport(ctx, client, config.ServerURL, report)
		if requestResult.err != nil || requestResult.status != http.StatusAccepted ||
			!requestResult.receipt.Accepted || requestResult.receipt.ResyncRequired {
			return result, fmt.Errorf("hello warmup failed: %s", describeRequestFailure(requestResult))
		}
		result.WarmupHellos++
	}

	total := int(math.Round(config.Duration.Seconds() * float64(config.ReportsPerSecond)))
	result.ScheduledReports = total
	jobs := make(chan scheduledReport, config.Workers*2)
	results := make(chan requestResult, config.Workers*2)
	var workers sync.WaitGroup
	for range config.Workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				lag := time.Since(job.due)
				requestResult := submitReport(ctx, client, config.ServerURL, job.report)
				requestResult.schedulerLag = lag
				results <- requestResult
			}
		}()
	}

	result.StartedAt = time.Now().UTC()
	go func() {
		defer close(jobs)
		var batch []domain.ReportEnvelope
		batchIndex := -1
		for index := range total {
			currentBatch := index / fixtures.DefaultScaleNodeCount
			if currentBatch != batchIndex {
				batchIndex = currentBatch
				sequence := int64(currentBatch + 2)
				batchAt := result.StartedAt.Add(time.Duration(currentBatch) * 2 * time.Second)
				batch = scenario.SteadyReports(batchAt, sequence)
			}
			due := result.StartedAt.Add(time.Duration(index) * time.Second / time.Duration(config.ReportsPerSecond))
			if wait := time.Until(due); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			jobs <- scheduledReport{report: batch[index%len(batch)], due: due}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	latencies := make([]time.Duration, 0, total)
	schedulerLags := make([]time.Duration, 0, total)
	for requestResult := range results {
		latencies = append(latencies, requestResult.latency)
		schedulerLags = append(schedulerLags, requestResult.schedulerLag)
		switch {
		case requestResult.err != nil:
			result.RequestErrors++
		case requestResult.status != http.StatusAccepted:
			result.HTTPFailures++
			if requestResult.status == http.StatusInternalServerError {
				result.HTTP500s++
			}
		case !requestResult.receipt.Accepted || requestResult.receipt.ResyncRequired:
			result.RejectedReceipts++
		default:
			result.AcceptedReports++
		}
	}
	result.FinishedAt = time.Now().UTC()
	result.CompletionDurationMS = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	if result.CompletionDurationMS > 0 {
		result.CompletionRPS = float64(result.AcceptedReports) / (float64(result.CompletionDurationMS) / 1000)
	}
	result.Ingest = Summarize(latencies)
	if len(schedulerLags) > 0 {
		result.SchedulerMaxLagMS = durationMS(maxDuration(schedulerLags))
	}

	if err := probeAPIs(ctx, client, config, &result); err != nil {
		result.Failures = append(result.Failures, err.Error())
	}
	evaluate(config, &result)
	if !result.Passed {
		return result, errors.New(strings.Join(result.Failures, "; "))
	}
	return result, nil
}

func validateConfig(config Config) error {
	if _, err := url.ParseRequestURI(config.ServerURL); err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}
	if config.Duration <= 0 || config.ReportsPerSecond <= 0 || config.Workers <= 0 ||
		config.RequestTimeout <= 0 || config.APISamples <= 0 {
		return errors.New("duration, reports per second, workers, timeout, and API samples must be positive")
	}
	return nil
}

func submitReport(ctx context.Context, client *http.Client, serverURL string, report domain.ReportEnvelope) requestResult {
	payload, err := json.Marshal(report)
	if err != nil {
		return requestResult{err: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(serverURL, "/")+"/api/v1/reports", bytes.NewReader(payload))
	if err != nil {
		return requestResult{err: err}
	}
	request.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := client.Do(request)
	result := requestResult{latency: time.Since(started), err: err}
	if err != nil {
		return result
	}
	defer response.Body.Close()
	result.status = response.StatusCode
	if response.StatusCode == http.StatusAccepted {
		result.err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result.receipt)
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	}
	return result
}

func probeAPIs(ctx context.Context, client *http.Client, config Config, result *Result) error {
	var topology domain.Topology
	topologyLatencies, err := sampleJSON(ctx, client, config.ServerURL+"/api/v1/topology", config.APISamples, &topology)
	if err != nil {
		return fmt.Errorf("topology probe: %w", err)
	}
	result.Topology = Summarize(topologyLatencies)
	result.TopologyNodes = len(topology.Nodes)
	result.TopologyEdges = len(topology.Edges)

	var page domain.HistoryEdgePage
	historyURL := config.ServerURL + "/api/v1/history/edges?window=1h&limit=100"
	historyLatencies, err := sampleJSON(ctx, client, historyURL, config.APISamples, &page)
	if err != nil {
		return fmt.Errorf("history list probe: %w", err)
	}
	result.HistoryList = Summarize(historyLatencies)
	if len(page.Edges) == 0 {
		return errors.New("history list probe returned no edges")
	}

	var detail domain.EdgeHistory
	detailURL := config.ServerURL + "/api/v1/history/edges/" + url.PathEscape(page.Edges[0].EdgeID) + "?window=1h"
	detailLatencies, err := sampleJSON(ctx, client, detailURL, config.APISamples, &detail)
	if err != nil {
		return fmt.Errorf("history detail probe: %w", err)
	}
	result.HistoryDetail = Summarize(detailLatencies)
	return nil
}

func sampleJSON(ctx context.Context, client *http.Client, endpoint string, samples int, target any) ([]time.Duration, error) {
	latencies := make([]time.Duration, 0, samples)
	for range samples {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		started := time.Now()
		response, err := client.Do(request)
		latencies = append(latencies, time.Since(started))
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("GET %s returned %s", endpoint, response.Status)
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(target); err != nil {
			response.Body.Close()
			return nil, err
		}
		response.Body.Close()
	}
	return latencies, nil
}

func Summarize(values []time.Duration) LatencySummary {
	if len(values) == 0 {
		return LatencySummary{}
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return LatencySummary{
		Samples: len(sorted),
		P50MS:   durationMS(percentile(sorted, 0.50)),
		P95MS:   durationMS(percentile(sorted, 0.95)),
		P99MS:   durationMS(percentile(sorted, 0.99)),
		MaxMS:   durationMS(sorted[len(sorted)-1]),
	}
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func durationMS(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}

func maxDuration(values []time.Duration) time.Duration {
	var maximum time.Duration
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func evaluate(config Config, result *Result) {
	addFailure := func(condition bool, format string, arguments ...any) {
		if condition {
			result.Failures = append(result.Failures, fmt.Sprintf(format, arguments...))
		}
	}
	addFailure(result.AcceptedReports != result.ScheduledReports,
		"accepted %d of %d scheduled reports", result.AcceptedReports, result.ScheduledReports)
	addFailure(result.RequestErrors != 0, "%d request errors", result.RequestErrors)
	addFailure(result.HTTPFailures != 0, "%d HTTP failures (%d status 500)", result.HTTPFailures, result.HTTP500s)
	addFailure(result.RejectedReceipts != 0, "%d rejected or resync receipts", result.RejectedReceipts)
	addFailure(result.Ingest.P95MS > durationMS(config.IngestP95Limit),
		"ingest p95 %.3fms exceeds %.3fms", result.Ingest.P95MS, durationMS(config.IngestP95Limit))
	addFailure(result.Ingest.P99MS > durationMS(config.IngestP99Limit),
		"ingest p99 %.3fms exceeds %.3fms", result.Ingest.P99MS, durationMS(config.IngestP99Limit))
	addFailure(result.SchedulerMaxLagMS > durationMS(config.SchedulerLagLimit),
		"scheduler max lag %.3fms exceeds %.3fms", result.SchedulerMaxLagMS, durationMS(config.SchedulerLagLimit))
	addFailure(result.TopologyNodes != fixtures.DefaultScaleNodeCount || result.TopologyEdges != fixtures.DefaultScaleEdgeCount,
		"topology has %d nodes/%d edges, want 250/1000", result.TopologyNodes, result.TopologyEdges)
	addFailure(result.Topology.P95MS > durationMS(config.TopologyP95Limit),
		"topology p95 %.3fms exceeds %.3fms", result.Topology.P95MS, durationMS(config.TopologyP95Limit))
	addFailure(result.HistoryList.P95MS > durationMS(config.HistoryP95Limit),
		"history list p95 %.3fms exceeds %.3fms", result.HistoryList.P95MS, durationMS(config.HistoryP95Limit))
	addFailure(result.HistoryDetail.P95MS > durationMS(config.HistoryP95Limit),
		"history detail p95 %.3fms exceeds %.3fms", result.HistoryDetail.P95MS, durationMS(config.HistoryP95Limit))
	result.Passed = len(result.Failures) == 0
}

func describeRequestFailure(result requestResult) string {
	if result.err != nil {
		return result.err.Error()
	}
	return fmt.Sprintf("status=%d accepted=%t resyncRequired=%t", result.status, result.receipt.Accepted, result.receipt.ResyncRequired)
}
