package ingestion

import (
	"context"
	"errors"
	"testing"

	"search-engine/internal/domain"
	"search-engine/internal/provider"
)

// fakeWriter/fakeIndexer are test doubles for the ContentWriter/Indexer
// interfaces — they let us test in isolation "in what order, under what
// conditions" Job.Reconcile and Job.runOne call what, without ever
// touching a real Mongo/Elasticsearch. These are the actual tests that
// prove the BEHAVIOR of the Mongo↔ES consistency fix (indexed flag +
// reconciliation, see the package comment on domain.Content).
type fakeWriter struct {
	upsertCalls [][]domain.Content
	markCalls   [][]domain.Content
	unindexed   []domain.Content
	upsertErr   error
	markErr     error
	findErr     error
}

func (f *fakeWriter) UpsertMany(ctx context.Context, items []domain.Content) error {
	f.upsertCalls = append(f.upsertCalls, items)
	return f.upsertErr
}

func (f *fakeWriter) MarkIndexed(ctx context.Context, items []domain.Content) error {
	f.markCalls = append(f.markCalls, items)
	return f.markErr
}

func (f *fakeWriter) FindUnindexed(ctx context.Context, limit int) ([]domain.Content, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.unindexed, nil
}

type fakeIndexer struct {
	indexCalls [][]domain.Content
	indexErr   error
}

func (f *fakeIndexer) IndexMany(ctx context.Context, items []domain.Content) error {
	f.indexCalls = append(f.indexCalls, items)
	return f.indexErr
}

func TestJob_Reconcile_NoPending(t *testing.T) {
	writer := &fakeWriter{unindexed: nil}
	indexer := &fakeIndexer{}
	job := New(writer, indexer)

	job.Reconcile(context.Background())

	if len(indexer.indexCalls) != 0 {
		t.Errorf("IndexMany should never be called when there are no pending records, was called %d times", len(indexer.indexCalls))
	}
	if len(writer.markCalls) != 0 {
		t.Errorf("MarkIndexed should never be called when there are no pending records, was called %d times", len(writer.markCalls))
	}
}

func TestJob_Reconcile_SuccessMarksIndexed(t *testing.T) {
	pending := []domain.Content{{ExternalID: "a", Provider: "p1"}, {ExternalID: "b", Provider: "p1"}}
	writer := &fakeWriter{unindexed: pending}
	indexer := &fakeIndexer{}
	job := New(writer, indexer)

	job.Reconcile(context.Background())

	if len(indexer.indexCalls) != 1 {
		t.Fatalf("IndexMany should have been called once, was called %d times", len(indexer.indexCalls))
	}
	if len(indexer.indexCalls[0]) != 2 {
		t.Errorf("IndexMany should have received 2 records, received %d", len(indexer.indexCalls[0]))
	}
	if len(writer.markCalls) != 1 {
		t.Fatalf("MarkIndexed should have been called once after successful indexing, was called %d times", len(writer.markCalls))
	}
	if len(writer.markCalls[0]) != 2 {
		t.Errorf("MarkIndexed should have received 2 records, received %d", len(writer.markCalls[0]))
	}
}

// TestJob_Reconcile_IndexFailure_DoesNotMark is the HEART of the
// consistency guarantee: if the write to ES fails, the "indexed" flag in
// Mongo must NEVER be updated — otherwise a record would be assumed to be
// "indexed" when it actually isn't in ES, would never be retried again,
// and the data would remain permanently "lost" (unsearchable).
func TestJob_Reconcile_IndexFailure_DoesNotMark(t *testing.T) {
	pending := []domain.Content{{ExternalID: "a", Provider: "p1"}}
	writer := &fakeWriter{unindexed: pending}
	indexer := &fakeIndexer{indexErr: errors.New("es is down")}
	job := New(writer, indexer)

	job.Reconcile(context.Background())

	if len(writer.markCalls) != 0 {
		t.Errorf("MarkIndexed should NEVER be called when IndexMany fails, was called %d times", len(writer.markCalls))
	}
}

func TestJob_Reconcile_FindUnindexedError_DoesNotPanic(t *testing.T) {
	writer := &fakeWriter{findErr: errors.New("mongo unreachable")}
	indexer := &fakeIndexer{}
	job := New(writer, indexer)

	// This call is expected to just log and return silently — if a panic
	// occurs, the test framework will already mark it as failed.
	job.Reconcile(context.Background())

	if len(indexer.indexCalls) != 0 {
		t.Errorf("IndexMany should never be called when FindUnindexed returns an error")
	}
}

// stubProvider is a test double for the provider.Provider interface.
type stubProvider struct {
	name  string
	items []domain.Content
	err   error
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Fetch(ctx context.Context) ([]domain.Content, error) {
	return s.items, s.err
}

func TestJob_RunOne_SuccessMarksIndexed(t *testing.T) {
	// DELIBERATE NOTE: provider.Register writes to a global registry (see
	// internal/provider/provider.go). We use a UNIQUE provider name in
	// each test to prevent leakage between tests.
	p := &stubProvider{name: "test-runone-success", items: []domain.Content{{ExternalID: "x", Provider: "test-runone-success"}}}
	provider.Register(p)

	writer := &fakeWriter{}
	indexer := &fakeIndexer{}
	job := New(writer, indexer)

	if err := job.RunOne(context.Background(), p.name); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(writer.upsertCalls) != 1 {
		t.Errorf("UpsertMany should have been called once, was called %d times", len(writer.upsertCalls))
	}
	if len(indexer.indexCalls) != 1 {
		t.Errorf("IndexMany should have been called once, was called %d times", len(indexer.indexCalls))
	}
	if len(writer.markCalls) != 1 {
		t.Errorf("MarkIndexed should have been called once after successful indexing, was called %d times", len(writer.markCalls))
	}
}

// TestJob_RunOne_IndexFailure_LeavesUnmarked is the runOne (normal
// ingestion flow) side counterpart of the "dual write" scenario: if the
// Mongo write succeeds but ES indexing fails, MarkIndexed should NEVER be
// called — the record should stay "indexed=false" (the default
// zero-value) so Reconcile can pick it up on the next round.
func TestJob_RunOne_IndexFailure_LeavesUnmarked(t *testing.T) {
	p := &stubProvider{name: "test-runone-indexfail", items: []domain.Content{{ExternalID: "y", Provider: "test-runone-indexfail"}}}
	provider.Register(p)

	writer := &fakeWriter{}
	indexer := &fakeIndexer{indexErr: errors.New("es is down")}
	job := New(writer, indexer)

	if err := job.RunOne(context.Background(), p.name); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(writer.upsertCalls) != 1 {
		t.Errorf("the write to Mongo should have been attempted even with ES down (UpsertMany once), was called %d times", len(writer.upsertCalls))
	}
	if len(writer.markCalls) != 0 {
		t.Errorf("MarkIndexed should NOT have been called while indexing failed, was called %d times", len(writer.markCalls))
	}
}

func TestJob_RunOne_ProviderFetchFailure_SkipsWrite(t *testing.T) {
	p := &stubProvider{name: "test-runone-fetchfail", err: errors.New("provider 503")}
	provider.Register(p)

	writer := &fakeWriter{}
	indexer := &fakeIndexer{}
	job := New(writer, indexer)

	if err := job.RunOne(context.Background(), p.name); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(writer.upsertCalls) != 0 {
		t.Errorf("UpsertMany should never be called when the provider fetch fails, was called %d times", len(writer.upsertCalls))
	}
}

func TestJob_RunOne_UnknownProvider_ReturnsError(t *testing.T) {
	writer := &fakeWriter{}
	indexer := &fakeIndexer{}
	job := New(writer, indexer)

	err := job.RunOne(context.Background(), "never-registered-provider")
	if err == nil {
		t.Fatal("expected an error for a provider that isn't registered")
	}
}
