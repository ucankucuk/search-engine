// Package ingestion manages the scheduled (or manually triggered) data
// collection flow: it calls every registered provider and writes the
// result to the storage layer. This package doesn't know whether
// providers are JSON or XML (it only talks through the provider.Provider
// interface), and doesn't know whether storage is Mongo or something else
// (it only talks through the storage.ContentWriter interface).
package ingestion

import (
	"context"
	"log/slog"

	"search-engine/internal/domain"
	"search-engine/internal/observability/metrics"
	"search-engine/internal/provider"
)

// ContentWriter is the minimum contract the ingestion job needs to
// persist data. Its concrete implementation is provided by
// internal/storage/mongo; the job never knows about that.
//
// MarkIndexed/FindUnindexed: the read/write side of the simplified outbox
// pattern for Mongo↔ES consistency (see the package comment on
// domain.Content.Indexed). We put these on ContentWriter (not on Indexer)
// because "is it indexed" is bookkeeping information WE keep —
// Elasticsearch itself has no idea about it, only Mongo knows.
type ContentWriter interface {
	UpsertMany(ctx context.Context, items []domain.Content) error
	MarkIndexed(ctx context.Context, items []domain.Content) error
	FindUnindexed(ctx context.Context, limit int) ([]domain.Content, error)
}

// Indexer is the separate contract used to reflect written data into the
// search index (Elasticsearch) — deliberately kept separate from
// storage.Writer, because "save" and "make searchable" are two different
// responsibilities.
type Indexer interface {
	IndexMany(ctx context.Context, items []domain.Content) error
}

type Job struct {
	writer  ContentWriter
	indexer Indexer
}

func New(writer ContentWriter, indexer Indexer) *Job {
	return &Job{writer: writer, indexer: indexer}
}

// reconcileBatchSize is the maximum number of "unindexed" records retried
// in a single Reconcile round — see Reconcile's own comment.
const reconcileBatchSize = 1000

// RunAll runs all registered providers in sequence (or in parallel; see
// note). One provider's failure doesn't block the others from running —
// this is the ingestion-level counterpart of the circuit breaker's
// principle that "a single provider crashing shouldn't crash the system".
//
// Reconcile() is called at the end: this is a step COMPLETELY independent
// of the provider fetches — it also retries records from past rounds that
// were written to Mongo but couldn't be indexed into ES (left with
// indexed=false), riding on the rhythm of this ingestion loop
// (INGEST_INTERVAL_SECONDS).
func (j *Job) RunAll(ctx context.Context) {
	for _, p := range provider.All() {
		j.runOne(ctx, p)
	}
	j.Reconcile(ctx)
}

// Reconcile is the "consumer" side of the simplified outbox pattern for
// Mongo↔ES consistency (see the package comment on domain.Content.Indexed).
// If IndexMany fails during runOne's normal flow, the record stays in
// Mongo with indexed=false; this method finds such records and tries to
// write them to ES again. This way, "I wrote to Mongo but it never made
// it to ES" is NOT a permanent state, but at most a temporary window of
// inconsistency lasting until the next ingestion round (a few
// seconds/minutes by default) — not data that silently disappears
// forever.
func (j *Job) Reconcile(ctx context.Context) {
	pending, err := j.writer.FindUnindexed(ctx, reconcileBatchSize)
	if err != nil {
		slog.Error("reconcile: failed to read unindexed records", "error", err)
		return
	}
	// PendingUnindexed reflects the raw backlog size OBSERVED in this
	// round, BEFORE the indexing attempt — whether the attempt succeeds
	// or fails, this number answers "what was the state a moment ago".
	// Because of the reconcileBatchSize (1000) cap, the actual backlog
	// may be larger; this metric says "at least this many", not "exactly
	// this many".
	metrics.PendingUnindexed.Set(float64(len(pending)))
	if len(pending) == 0 {
		return // everything is already indexed — the expected, common case
	}

	if err := j.indexer.IndexMany(ctx, pending); err != nil {
		slog.Error("reconcile: re-indexing failed, will retry next round",
			"pending_count", len(pending), "error", err)
		return
	}
	if err := j.writer.MarkIndexed(ctx, pending); err != nil {
		// NOTE: In this case the write to ES SUCCEEDED but we failed to
		// update the flag in Mongo — the same records will be
		// UNNECESSARILY re-indexed on the next round. Harmless (IndexMany
		// is idempotent, the ES document is overwritten by ID) but
		// inefficient; worth logging.
		slog.Error("reconcile: failed to update indexed flag (data was written to ES, will retry next round)",
			"pending_count", len(pending), "error", err)
		return
	}
	slog.Info("reconcile: pending records indexed successfully", "count", len(pending))
}

// RunOne triggers a single provider by name — used to serve a targeted
// manual refresh request like /refresh?provider=xxx.
func (j *Job) RunOne(ctx context.Context, name string) error {
	p, err := provider.Get(name)
	if err != nil {
		return err
	}
	j.runOne(ctx, p)
	return nil
}

func (j *Job) runOne(ctx context.Context, p provider.Provider) {
	items, err := p.Fetch(ctx)
	if err != nil {
		slog.Error("provider fetch failed", "provider", p.Name(), "error", err)
		return // the circuit breaker is already engaged; one provider's failure doesn't stop the others
	}
	if err := j.writer.UpsertMany(ctx, items); err != nil {
		slog.Error("failed to write to mongo", "provider", p.Name(), "error", err)
		return
	}
	if err := j.indexer.IndexMany(ctx, items); err != nil {
		// WARNING: We return early here but data is NOT lost — UpsertMany
		// already wrote these records with indexed=false (see the
		// zero-value of the domain.Content.Indexed field). Reconcile()
		// will automatically pick these up and retry on the next
		// ingestion round; that's why we do NOT write retry/backoff logic
		// here — that job is already delegated to Reconcile.
		slog.Error("elasticsearch indexing failed, reconcile will retry next round",
			"provider", p.Name(), "error", err)
		return
	}
	if err := j.writer.MarkIndexed(ctx, items); err != nil {
		// Same rationale: the write to ES succeeded but the flag couldn't
		// be updated — the record stays indexed=false, and Reconcile
		// re-indexes it (at the cost of a harmless unnecessary repeat).
		slog.Error("failed to update indexed flag, reconcile will retry next round",
			"provider", p.Name(), "error", err)
		return
	}
	slog.Info("provider fetch completed", "provider", p.Name(), "item_count", len(items))
}
