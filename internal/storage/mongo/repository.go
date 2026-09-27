// Package mongo encapsulates access to MongoDB. This package is the
// "system of record" layer — the single durable, persistent copy of the
// raw and normalized content is kept here (see the "why Mongo, why not
// Elasticsearch" discussion in the architecture doc).
//
// Other packages (ingestion, search) don't use this package's concrete
// types, but the narrow interfaces they define themselves (like
// ingestion.ContentWriter) — this package simply satisfies those
// interfaces (implicit interface satisfaction, Go's idiomatic path to
// dependency inversion).
package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"search-engine/internal/domain"
)

type Repository struct {
	collection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{collection: db.Collection("contents")}
}

// batchSize is the maximum number of models sent in a single BulkWrite
// call. Instead of sending thousands of records in ONE giant request, we
// split them into reasonable chunks — both to stay under
// memory/request-size limits and to keep a failure in one chunk from
// affecting the other chunks (each chunk is its own independent
// BulkWrite call).
const batchSize = 1000

// UpsertMany performs an upsert keyed on external_id+provider — when the
// same content is fetched a second time from the same provider (which
// happens every time the job fires), it results in an update, not a new
// record. This guarantees idempotency in retry/re-trigger scenarios.
//
// IMPORTANT: The previous implementation wrote each record one at a time
// with UpdateOne — for thousands of records that meant thousands of
// separate round-trips (tens of seconds for a provider with 20,000
// records), and a single record's failure would stop ALL remaining
// records from being written (the function returned early on the first
// error). With BulkWrite, the whole batch is written in a SINGLE request
// (and, with ordered:false, independently of one another as well).
func (r *Repository) UpsertMany(ctx context.Context, items []domain.Content) error {
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}

		models := make([]mongo.WriteModel, 0, end-start)
		for _, item := range items[start:end] {
			filter := bson.M{"external_id": item.ExternalID, "provider": item.Provider}
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(filter).
				SetUpdate(bson.M{"$set": item}).
				SetUpsert(true))
		}

		// ordered:false → an error on one document doesn't block the
		// other documents in the batch from being written; at worst that
		// one document is skipped, the rest are written successfully.
		opts := options.BulkWrite().SetOrdered(false)
		if _, err := r.collection.BulkWrite(ctx, models, opts); err != nil {
			return err
		}
	}
	return nil
}

// FindUnindexed fetches the records marked indexed=false (i.e. written to
// Mongo but not yet successfully reflected into Elasticsearch).
// ingestion.Job.Reconcile calls this on every ingestion round — see the
// package comment on domain.Content.Indexed (the simplified outbox
// pattern). limit is the maximum number of records returned at once:
// reconciliation is a "best-effort" loop, it works through a huge backlog
// in small chunks instead of trying to pull it all at once in a single
// round — the next round picks up the rest.
func (r *Repository) FindUnindexed(ctx context.Context, limit int) ([]domain.Content, error) {
	filter := bson.M{"indexed": false}
	opts := options.Find().SetLimit(int64(limit))
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []domain.Content
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// MarkIndexed marks the given records as indexed=true, indexed_at=now —
// called AFTER a successful indexing into ES (this is never called if
// indexing failed, whether in ingestion.Job.runOne's normal flow or in
// Reconcile; the record stays indexed=false and is retried on the next
// Reconcile round — a self-healing loop).
func (r *Repository) MarkIndexed(ctx context.Context, items []domain.Content) error {
	now := time.Now()
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}

		models := make([]mongo.WriteModel, 0, end-start)
		for _, item := range items[start:end] {
			filter := bson.M{"external_id": item.ExternalID, "provider": item.Provider}
			update := bson.M{"$set": bson.M{"indexed": true, "indexed_at": now}}
			models = append(models, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update))
		}

		opts := options.BulkWrite().SetOrdered(false)
		if _, err := r.collection.BulkWrite(ctx, models, opts); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, provider, externalID string) (*domain.Content, error) {
	var content domain.Content
	filter := bson.M{"external_id": externalID, "provider": provider}
	if err := r.collection.FindOne(ctx, filter).Decode(&content); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrContentNotFound
		}
		return nil, err
	}
	return &content, nil
}
