package forwarder

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

// Use a separate database: storage integration tests truncate their tables and
// go test runs different packages concurrently. The CI PostgreSQL role owns DBs.
func openForwarderTestDB(t *testing.T) *storage.DB {
	t.Helper()
	for _, name := range []string{"TEST_POSTGRES_HOST", "TEST_POSTGRES_PORT", "TEST_POSTGRES_USER", "TEST_POSTGRES_PASSWORD", "TEST_POSTGRES_DB"} {
		if os.Getenv(name) == "" {
			t.Skip("PostgreSQL integration test environment is not configured")
		}
	}
	config := storage.Config{Host: os.Getenv("TEST_POSTGRES_HOST"), Port: os.Getenv("TEST_POSTGRES_PORT"), User: os.Getenv("TEST_POSTGRES_USER"), Password: os.Getenv("TEST_POSTGRES_PASSWORD"), Name: os.Getenv("TEST_POSTGRES_DB"), SSLMode: "disable"}
	endpoint := url.URL{Scheme: "postgres", User: url.UserPassword(config.User, config.Password), Host: net.JoinHostPort(config.Host, config.Port), Path: config.Name, RawQuery: "sslmode=disable"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	name := "webhookhub_forwarder_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Error(err)
		}
	})
	config.Name = name
	db, err := storage.Open(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestForwardFinalizesAllOutcomes(t *testing.T) {
	db := openForwarderTestDB(t)
	originalClient := deliveryClient
	t.Cleanup(func() { deliveryClient = originalClient })
	for _, scenario := range []string{"success", "retry", "dead_letter", "cancelled", "skipped"} {
		t.Run(scenario, func(t *testing.T) {
			hook := model.Webhook{Source: scenario, Status: "pending", ReceivedAt: time.Now()}
			if err := db.Save(&hook); err != nil {
				t.Fatal(err)
			}
			if scenario != "skipped" {
				attempts := 3
				if scenario == "dead_letter" {
					attempts = 1
				}
				if err := db.SaveForwardingRule(model.ForwardingRule{Source: scenario, Target: "https://example.com", RetryMaxAttempts: attempts}); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now()
			claims, err := db.ClaimDeliverableWebhooks(context.Background(), 1, now, now.Add(time.Minute))
			if err != nil || len(claims) != 1 || claims[0].ID != hook.ID {
				t.Fatalf("claim: %+v %v", claims, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			called := false
			deliveryClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				called = true
				if scenario == "skipped" {
					t.Fatal("skipped webhook was sent")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > DefaultDeliveryTimeout || time.Until(deadline) < 4*time.Second {
					t.Fatal("Forward did not normalize zero timeout")
				}
				if scenario == "cancelled" {
					cancel()
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				status := 200
				if scenario == "retry" || scenario == "dead_letter" {
					status = 500
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("response"))}, nil
			})}
			err = Forward(ctx, db, &claims[0], 0)
			if scenario == "cancelled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if called != (scenario != "skipped") {
				t.Fatal("wrong outbound delivery behavior")
			}
			wantWebhook := map[string]string{"success": "success", "retry": "retrying", "dead_letter": "dead_lettered", "cancelled": "pending", "skipped": "skipped"}[scenario]
			wantAttempt := map[string]string{"success": "success", "retry": "failed", "dead_letter": "failed", "cancelled": "cancelled", "skipped": "skipped"}[scenario]
			stored, err := db.FindByID(int(hook.ID))
			if err != nil || stored.Status != wantWebhook || stored.DeliveryLeaseUntil != nil {
				t.Fatalf("webhook: %+v %v", stored, err)
			}
			attempts, err := db.DeliveryAttemptsByWebhook(hook.ID)
			if err != nil || len(attempts) != 1 || attempts[0].Status != wantAttempt {
				t.Fatalf("attempts: %+v %v", attempts, err)
			}
			if err := db.DeleteWebhook(int(hook.ID)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestForwardRejectsExpiredClaimBeforeDatabaseAccess(t *testing.T) {
	expired := time.Now().Add(-time.Second)
	if err := Forward(context.Background(), nil, &model.Webhook{DeliveryLeaseUntil: &expired}, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired claim: %v", err)
	}
}
