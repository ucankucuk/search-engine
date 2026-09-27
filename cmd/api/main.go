// cmd/api is the application's single entry point (composition root). The
// only thing done here is: reading the config, wiring up all the
// dependencies (Mongo, ES, providers, services, handlers) and connecting
// them together. No business logic is written here — only "wiring".
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/sony/gobreaker"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"

	"search-engine/internal/config"
	"search-engine/internal/ingestion"
	"search-engine/internal/observability/logger"
	"search-engine/internal/observability/metrics"
	"search-engine/internal/provider"
	jsonprovider "search-engine/internal/provider/json"
	xmlprovider "search-engine/internal/provider/xml"
	"search-engine/internal/search"
	"search-engine/internal/storage/elastic"
	mongorepo "search-engine/internal/storage/mongo"
	transporthttp "search-engine/internal/transport/http"
	"search-engine/internal/transport/http/handler"
)

func main() {
	logger.Init(slog.LevelInfo)
	cfg := config.Load()

	// ctx is canceled when SIGTERM (the signal docker stop sends) or SIGINT
	// (Ctrl+C) is received. This single context both drives the background
	// loop of the ingestion scheduler (see ingestion.Scheduler.Start, which
	// was already listening to ctx.Done() — we deliberately designed it
	// this way BEFORE this change) and triggers when the HTTP server
	// switches to graceful shutdown. stop() is deferred so the signal
	// listener is cleaned up at the end of the program.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 1) Data layer
	mongoClient, err := mongo.Connect(ctx, mongooptions.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		slog.Error("failed to establish mongo connection", "error", err)
		return
	}
	repo := mongorepo.NewRepository(mongoClient.Database(cfg.MongoDatabase))

	esClient, err := elastic.NewClient(cfg.ElasticURL)
	if err != nil {
		slog.Error("failed to establish elasticsearch connection", "error", err)
		return
	}

	// 2) Providers register themselves from their own packages — main has
	//    no idea whether a given provider is JSON or XML.
	jsonprovider.Register(jsonprovider.Config{
		ProviderAURL: cfg.ProviderAURL,
		ProviderBURL: cfg.ProviderBURL,
		Timeout:      cfg.FetchTimeout,
	})
	xmlprovider.Register(xmlprovider.Config{
		URL:     cfg.ProviderXMLURL,
		Timeout: cfg.FetchTimeout,
	})

	// 3) Every provider is wrapped with the resilience layer — this loop
	//    doesn't change even if the number of providers grows.
	for _, p := range provider.All() {
		provider.NewResilient(p,
			provider.WithTimeout(cfg.FetchTimeout),
			provider.WithStateChange(func(name string, from, to gobreaker.State) {
				metrics.CircuitState.WithLabelValues(name).Set(float64(to))
				slog.Info("circuit breaker state changed", "provider", name, "from", from.String(), "to", to.String())
			}),
		)
	}

	// 4) Business logic services
	ingestJob := ingestion.New(repo, esClient)
	scheduler := ingestion.NewScheduler(ingestJob, cfg.IngestInterval)
	scheduler.Start(ctx)

	searchService := search.NewService(esClient)

	// 5) HTTP layer
	router := transporthttp.NewRouter(transporthttp.Handlers{
		Search:  handler.NewSearchHandler(searchService),
		Refresh: handler.NewRefreshHandler(ingestJob),
	}, cfg.FrontendOrigin, cfg.RateLimitRPS, cfg.RateLimitBurst)

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: router,
	}

	// The server runs in a SEPARATE goroutine — this lets the main goroutine
	// wait for the signal below with <-ctx.Done(). ListenAndServe ALWAYS
	// returns http.ErrServerClosed on a normal shutdown (when Shutdown is
	// called) — this is expected behavior, not a real error; that's why it
	// is logged ONLY if there is some other error.
	go func() {
		slog.Info("starting server", "port", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped unexpectedly", "error", err)
		}
	}()

	// The main goroutine blocks here — until SIGINT/SIGTERM arrives.
	<-ctx.Done()
	slog.Info("shutdown signal received, starting graceful shutdown")

	// shutdownCtx: shutdown operations (finishing in-flight HTTP requests,
	// closing the Mongo connection) are given at most 15 seconds — if this
	// time is exceeded, they are forced closed. ctx (above, whose signal
	// has already been canceled) is NOT used here because it is already in
	// the "Done" state; a new, independent timeout context is needed.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server did not shut down cleanly", "error", err)
	}
	if err := mongoClient.Disconnect(shutdownCtx); err != nil {
		slog.Error("failed to close mongo connection", "error", err)
	}
	slog.Info("application shut down cleanly")
}
