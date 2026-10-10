package storage

import (
	"context"
	"math"
	"testing"
	"time"

	"webhookhub/internal/model"
)

func TestDeliveryMetricsAggregatesAllStatuses(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	metrics, err := db.DeliveryMetrics(ctx)
	if err != nil || metrics.TotalWebhooks != 0 || metrics.TotalAttempts != 0 || metrics.SuccessRate != 0 || len(metrics.SourceBreakdown) != 0 {
		t.Fatalf("empty metrics: %+v %v", metrics, err)
	}
	now := time.Now()
	for _, status := range []string{"success", "dead_lettered"} {
		hook := model.Webhook{Source: "alpha", Status: status, ReceivedAt: now, DeadLetteredAt: &now, Payload: []byte("payload"), Headers: "headers", Response: []byte("response")}
		if err := db.Save(ctx, &hook); err != nil {
			t.Fatal(err)
		}
	}
	for source, statuses := range map[string][]string{
		"alpha": {"success", "success", "failed", "pending", "skipped", "cancelled", "interrupted"},
		"beta":  {"failed", "failed", "skipped", "skipped"},
	} {
		for _, status := range statuses {
			if _, err := createAttemptFixture(db, &model.DeliveryAttempt{WebhookID: 1, Source: source, Status: status, StartedAt: now, ResponseBody: []byte("response"), ResponseHeaders: "headers", ResponseCaptured: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	metrics, err = db.DeliveryMetrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TotalWebhooks != 2 || metrics.DeadLetterCount != 1 || metrics.TotalAttempts != 11 || metrics.SuccessCount != 2 || metrics.FailedCount != 3 || metrics.PendingCount != 1 || metrics.SkippedCount != 3 || metrics.SuccessRate != 40 {
		t.Fatalf("incorrect totals: %+v", metrics)
	}
	if len(metrics.SourceBreakdown) != 2 {
		t.Fatalf("incorrect sources: %+v", metrics.SourceBreakdown)
	}
	alpha, beta := metrics.SourceBreakdown[0], metrics.SourceBreakdown[1]
	if alpha.Source != "alpha" || alpha.Attempts != 7 || alpha.Success != 2 || alpha.Failed != 1 || alpha.Pending != 1 || alpha.Skipped != 1 || math.Abs(alpha.SuccessRate-200.0/3) > 0.0001 {
		t.Fatalf("incorrect alpha metrics: %+v", alpha)
	}
	if beta.Source != "beta" || beta.Attempts != 4 || beta.Failed != 2 || beta.Skipped != 2 || beta.SuccessRate != 0 {
		t.Fatalf("incorrect beta metrics: %+v", beta)
	}
	if len(metrics.RecentFailures) != 3 || len(metrics.RecentDeadLetters) != 1 {
		t.Fatalf("recent records: %+v", metrics)
	}
	for _, attempt := range metrics.RecentFailures {
		if len(attempt.ResponseBody) != 0 || attempt.ResponseHeaders != "" {
			t.Fatal("metrics loaded attempt contents")
		}
	}
	for _, hook := range metrics.RecentDeadLetters {
		if len(hook.Payload) != 0 || hook.Headers != "" || len(hook.Response) != 0 {
			t.Fatal("metrics loaded webhook contents")
		}
	}
}
