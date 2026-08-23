package perfgate

import (
	"testing"
	"time"
)

func TestSummarizeUsesNearestRankPercentiles(t *testing.T) {
	values := make([]time.Duration, 100)
	for index := range values {
		values[index] = time.Duration(index+1) * time.Millisecond
	}
	summary := Summarize(values)
	if summary.Samples != 100 || summary.P50MS != 50 || summary.P95MS != 95 ||
		summary.P99MS != 99 || summary.MaxMS != 100 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestEvaluateEnforcesEveryThreshold(t *testing.T) {
	config := DefaultConfig()
	result := Result{
		ScheduledReports: 10, AcceptedReports: 9, RequestErrors: 1,
		HTTPFailures: 1, HTTP500s: 1, RejectedReceipts: 1,
		SchedulerMaxLagMS: 1001,
		Ingest:            LatencySummary{P95MS: 101, P99MS: 251},
		Topology:          LatencySummary{P95MS: 251},
		HistoryList:       LatencySummary{P95MS: 501},
		HistoryDetail:     LatencySummary{P95MS: 501},
		TopologyNodes:     249, TopologyEdges: 999,
	}
	evaluate(config, &result)
	if result.Passed || len(result.Failures) != 11 {
		t.Fatalf("result = %#v, want eleven failures", result)
	}
}
