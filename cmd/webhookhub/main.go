package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"webhookhub/internal/forwarder"
	"webhookhub/internal/handler"
	"webhookhub/internal/storage"
	"webhookhub/web"

	"github.com/joho/godotenv"
)

type appConfig struct {
	Database           storage.Config
	AdminEmail         string
	AdminPassword      string
	SessionKey         string
	CookieSecure       bool
	TrustProxyHeaders  bool
	Port               int
	MaxBodyBytes       int64
	DeliveryWorkers    int
	DeliveryPoll       time.Duration
	DeliveryLease      time.Duration
	ShutdownTimeout    time.Duration
	RetentionEnabled   bool
	RetentionDays      int
	RetentionInterval  time.Duration
	RetentionBatchSize int
}

func main() {
	if len(os.Args) > 1 {
		if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
			if err := runHealthcheck(); err != nil {
				log.Fatal(err)
			}
			return
		}
		log.Fatalf("usage: %s [healthcheck]", os.Args[0])
	}

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func runHealthcheck() error {
	port, err := positiveIntEnv("PORT", 8080)
	if err != nil || port > 65535 {
		return errors.New("PORT must be an integer from 1 to 65535")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	return checkReadiness(client, fmt.Sprintf("http://127.0.0.1:%d/readyz", port))
}

func checkReadiness(client *http.Client, endpoint string) error {
	response, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("readiness request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness request returned HTTP %d", response.StatusCode)
	}
	return nil
}

func run() error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("load .env: %v", err)
	}

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	db, err := storage.Open(config.Database)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	if err := db.EnsureAdmin(config.AdminEmail, config.AdminPassword); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}

	auth, err := handler.NewAuth(config.SessionKey, config.CookieSecure, config.TrustProxyHeaders)
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	deliveryWorkers := forwarder.StartWorkerPool(workerCtx, db, forwarder.WorkerConfig{
		Count:         config.DeliveryWorkers,
		PollInterval:  config.DeliveryPoll,
		LeaseDuration: config.DeliveryLease,
	})
	retentionWorkers := &sync.WaitGroup{}
	if config.RetentionEnabled {
		retentionWorkers = storage.StartRetentionWorker(
			workerCtx,
			db,
			config.RetentionDays,
			config.RetentionInterval,
			config.RetentionBatchSize,
		)
		log.Printf("retention cleanup enabled: keep %d days, run every %s, batch %d", config.RetentionDays, config.RetentionInterval, config.RetentionBatchSize)
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.Port),
		Handler:           routes(db, auth, config.MaxBodyBytes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	var runErr error
	select {
	case <-signalCtx.Done():
		log.Println("shutdown signal received")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("HTTP server stopped: %w", err)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful HTTP shutdown: %v", err)
		if runErr == nil {
			runErr = fmt.Errorf("graceful HTTP shutdown: %w", err)
		}
	}

	stopWorkers()
	waitForWorkers(shutdownCtx, deliveryWorkers, retentionWorkers)
	return runErr
}

func routes(db *storage.DB, auth *handler.Auth, maxBodyBytes int64) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(web.Static)))
	mux.HandleFunc("GET /healthz", handler.Health())
	mux.HandleFunc("GET /readyz", handler.Ready(db))
	mux.HandleFunc("GET /login", auth.Login(db))
	mux.HandleFunc("POST /login", auth.Login(db))
	mux.HandleFunc("POST /hook/{source}", handler.ReceiveWebhook(db, maxBodyBytes))

	protected := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.RequireAuth(h)
	}
	protectedMutation := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.RequireAuth(auth.RequireCSRF(h))
	}

	mux.HandleFunc("GET /{$}", protected(handler.ServeIndex()))
	mux.HandleFunc("GET /dashboard", protected(handler.DashboardUI(db)))
	mux.HandleFunc("GET /dlq", protected(handler.DLQUI(db)))
	mux.HandleFunc("GET /api/webhooks", protected(handler.ListWebhooks(db)))
	mux.HandleFunc("POST /api/webhooks/replay", protectedMutation(handler.ReplayWebhook(db)))
	mux.HandleFunc("POST /api/webhooks/delete", protectedMutation(handler.DeleteWebhook(db)))
	mux.HandleFunc("GET /partials/metrics", protected(handler.DeliveryMetricsPartial(db)))
	mux.HandleFunc("GET /partials/webhooks", protected(handler.WebhookPartial(db)))
	mux.HandleFunc("GET /partials/webhook/{id}", protected(handler.InspectWebhook(db)))
	mux.HandleFunc("GET /webhooks/{id}", protected(handler.InspectWebhook(db)))
	mux.HandleFunc("GET /webhooks/{id}/attempts/{attemptID}", protected(handler.InspectDeliveryAttempt(db)))
	mux.HandleFunc("GET /webhooks/{id}/payload", protected(handler.DownloadWebhookPayload(db)))
	mux.HandleFunc("GET /partials/webhook/{id}/delivery", protected(handler.InspectDeliveryPartial(db)))
	mux.HandleFunc("GET /forwarding", protected(handler.ForwardingUI(db)))
	mux.HandleFunc("POST /forwarding/save", protectedMutation(handler.SaveForwardingRule(db)))
	mux.HandleFunc("GET /forwarding/edit", protected(handler.EditForwardingForm(db)))
	mux.HandleFunc("POST /forwarding/update", protectedMutation(handler.UpdateForwardingRule(db)))
	mux.HandleFunc("POST /forwarding/delete", protectedMutation(handler.DeleteForwardingRule(db)))
	mux.HandleFunc("POST /logout", protectedMutation(auth.Logout()))

	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func loadConfig() (appConfig, error) {
	var config appConfig
	var err error

	if config.Database.Host, err = requiredEnv("POSTGRES_HOST"); err != nil {
		return config, err
	}
	if config.Database.Port, err = requiredEnv("POSTGRES_PORT"); err != nil {
		return config, err
	}
	if config.Database.User, err = requiredEnv("POSTGRES_USER"); err != nil {
		return config, err
	}
	if config.Database.Password, err = requiredEnv("POSTGRES_PASSWORD"); err != nil {
		return config, err
	}
	if config.Database.Name, err = requiredEnv("POSTGRES_DB"); err != nil {
		return config, err
	}
	config.Database.SSLMode = envOrDefault("POSTGRES_SSLMODE", "disable")

	if config.AdminEmail, err = requiredEnv("ADMIN_EMAIL"); err != nil {
		return config, err
	}
	address, err := mail.ParseAddress(config.AdminEmail)
	if err != nil || address.Address != config.AdminEmail {
		return config, errors.New("ADMIN_EMAIL must be a valid email address")
	}
	if config.AdminPassword, err = requiredEnv("ADMIN_PASSWORD"); err != nil {
		return config, err
	}
	if len(config.AdminPassword) < 12 {
		return config, errors.New("ADMIN_PASSWORD must contain at least 12 characters")
	}
	if config.SessionKey, err = requiredEnv("SESSION_KEY"); err != nil {
		return config, err
	}
	if len(config.SessionKey) < 32 {
		return config, errors.New("SESSION_KEY must contain at least 32 characters")
	}

	if config.CookieSecure, err = boolEnv("COOKIE_SECURE", false); err != nil {
		return config, err
	}
	if config.TrustProxyHeaders, err = boolEnv("TRUST_PROXY_HEADERS", false); err != nil {
		return config, err
	}
	if config.Port, err = positiveIntEnv("PORT", 8080); err != nil || config.Port > 65535 {
		return config, errors.New("PORT must be an integer from 1 to 65535")
	}
	if config.MaxBodyBytes, err = positiveInt64Env("MAX_BODY_BYTES", 1<<20); err != nil {
		return config, err
	}
	if config.DeliveryWorkers, err = positiveIntEnv("DELIVERY_WORKERS", 4); err != nil {
		return config, err
	}
	if config.DeliveryPoll, err = positiveDurationEnv("DELIVERY_POLL_INTERVAL", time.Second); err != nil {
		return config, err
	}
	if config.DeliveryLease, err = positiveDurationEnv("DELIVERY_LEASE_DURATION", 30*time.Second); err != nil {
		return config, err
	}
	if config.DeliveryLease < 10*time.Second {
		return config, errors.New("DELIVERY_LEASE_DURATION must be at least 10s")
	}
	if config.ShutdownTimeout, err = positiveDurationEnv("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return config, err
	}

	if config.RetentionEnabled, err = boolEnv("RETENTION_ENABLED", false); err != nil {
		return config, err
	}
	if config.RetentionDays, err = positiveIntEnv("RETENTION_DAYS", 30); err != nil {
		return config, err
	}
	if config.RetentionInterval, err = positiveDurationEnv("RETENTION_INTERVAL", 24*time.Hour); err != nil {
		return config, err
	}
	if config.RetentionBatchSize, err = positiveIntEnv("RETENTION_BATCH_SIZE", 300); err != nil {
		return config, err
	}

	return config, nil
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func positiveIntEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func positiveInt64Env(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func positiveDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

type waitGroup interface {
	Wait()
}

func waitForWorkers(ctx context.Context, groups ...waitGroup) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, group := range groups {
			group.Wait()
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		log.Printf("worker shutdown timed out: %v", ctx.Err())
	}
}
