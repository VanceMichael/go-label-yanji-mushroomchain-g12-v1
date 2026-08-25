package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
)

type fakeOutbox struct {
	mu        sync.Mutex
	events    []domain.OutboxEvent
	completed []string
	failed    []string
	claimErr  error
}

func (f *fakeOutbox) CreateOutboxEvent(context.Context, domain.OutboxEvent) error {
	return errors.New("not implemented")
}
func (f *fakeOutbox) ClaimOutbox(ctx context.Context, owner string, now, until time.Time) (domain.OutboxEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.OutboxEvent{}, err
	}
	if f.claimErr != nil {
		return domain.OutboxEvent{}, f.claimErr
	}
	if len(f.events) == 0 {
		return domain.OutboxEvent{}, domain.ErrNotFound
	}
	event := f.events[0]
	f.events = f.events[1:]
	event.Status = domain.OutboxClaimed
	event.LeaseOwner = owner
	event.LeaseUntil = &until
	event.Attempts++
	return event, nil
}
func (f *fakeOutbox) CompleteOutbox(_ context.Context, id, owner string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completed = append(f.completed, id+":"+owner)
	return nil
}
func (f *fakeOutbox) FailOutbox(_ context.Context, id, owner, message string, remaining int, next time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, id+":"+owner+":"+message)
	return nil
}

type recordingPublisher struct {
	mu       sync.Mutex
	types    []string
	payloads [][]byte
	err      error
}

func (p *recordingPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.types = append(p.types, eventType)
	p.payloads = append(p.payloads, append([]byte(nil), payload...))
	return p.err
}

func testWorker(repo *fakeOutbox, publisher *recordingPublisher, c *clock.Fixed) *OutboxWorker {
	return NewOutbox(repo, c, publisher, slog.New(slog.NewTextHandler(io.Discard, nil)), "worker-test", time.Millisecond, time.Minute)
}

func TestDrainPublishesAndCompletesAvailableEvents(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	repo := &fakeOutbox{events: []domain.OutboxEvent{{ID: "one", EventType: "settlement.created", Payload: []byte(`{"id":1}`), Status: domain.OutboxPending}, {ID: "two", EventType: "settlement.paid", Payload: []byte(`{"id":2}`), Status: domain.OutboxPending}}}
	publisher := &recordingPublisher{}
	worker := testWorker(repo, publisher, clock.NewFixed(now))
	worker.drain(context.Background())
	if len(publisher.types) != 2 || publisher.types[0] != "settlement.created" || publisher.types[1] != "settlement.paid" {
		t.Fatalf("published=%v", publisher.types)
	}
	if len(repo.completed) != 2 || repo.completed[0] != "one:worker-test" || repo.completed[1] != "two:worker-test" {
		t.Fatalf("completed=%v", repo.completed)
	}
	if len(repo.failed) != 0 {
		t.Fatalf("failed=%v", repo.failed)
	}
}

func TestDrainSchedulesRetryAfterPublishFailure(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	repo := &fakeOutbox{events: []domain.OutboxEvent{{ID: "one", EventType: "settlement.created", Payload: []byte(`{}`), Status: domain.OutboxPending}}}
	publisher := &recordingPublisher{err: errors.New("broker unavailable")}
	worker := testWorker(repo, publisher, clock.NewFixed(now))
	worker.drain(context.Background())
	if len(repo.failed) != 1 {
		t.Fatalf("failed=%v", repo.failed)
	}
	if len(repo.completed) != 0 {
		t.Fatalf("completed=%v", repo.completed)
	}
}

func TestDrainStopsWhenClaimFails(t *testing.T) {
	repo := &fakeOutbox{claimErr: errors.New("database unavailable")}
	publisher := &recordingPublisher{}
	worker := testWorker(repo, publisher, clock.NewFixed(time.Now()))
	worker.drain(context.Background())
	if len(publisher.types) != 0 || len(repo.completed) != 0 || len(repo.failed) != 0 {
		t.Fatalf("publisher=%v completed=%v failed=%v", publisher.types, repo.completed, repo.failed)
	}
}

func TestRunStopsOnContextCancellation(t *testing.T) {
	repo := &fakeOutbox{}
	publisher := &recordingPublisher{}
	worker := testWorker(repo, publisher, clock.NewFixed(time.Now()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { worker.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	worker.Wait()
}

func TestBackoffIsBoundedExponential(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{{-1, time.Second}, {0, time.Second}, {1, time.Second}, {2, 2 * time.Second}, {3, 4 * time.Second}, {6, 32 * time.Second}, {7, time.Minute}, {20, time.Minute}}
	for _, test := range tests {
		if got := backoff(test.attempt); got != test.want {
			t.Fatalf("backoff(%d)=%s want %s", test.attempt, got, test.want)
		}
	}
}

func TestLogPublisherHonorsCancellationAndConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	publisher := LogPublisher{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := publisher.Publish(ctx, "event", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	if err := (LogPublisher{}).Publish(context.Background(), "event", nil); err == nil {
		t.Fatal("nil logger accepted")
	}
	if err := publisher.Publish(context.Background(), "event", []byte(`{}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}
}
