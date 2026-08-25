package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/repository"
)

type Publisher interface {
	Publish(context.Context, string, []byte) error
}
type FuncPublisher func(context.Context, string, []byte) error

func (f FuncPublisher) Publish(ctx context.Context, t string, p []byte) error { return f(ctx, t, p) }

type OutboxWorker struct {
	repo            repository.Outbox
	clock           clock.Clock
	publisher       Publisher
	logger          *slog.Logger
	owner           string
	interval, lease time.Duration
	maxAttempts     int
	wg              sync.WaitGroup
}

func NewOutbox(repo repository.Outbox, c clock.Clock, p Publisher, logger *slog.Logger, owner string, interval, lease time.Duration) *OutboxWorker {
	return &OutboxWorker{repo: repo, clock: c, publisher: p, logger: logger, owner: owner, interval: interval, lease: lease, maxAttempts: 5}
}

func (w *OutboxWorker) Run(ctx context.Context) {
	w.wg.Add(1)
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *OutboxWorker) Wait() { w.wg.Wait() }

func (w *OutboxWorker) drain(ctx context.Context) {
	for index := 0; index < 20; index++ {
		if err := ctx.Err(); err != nil {
			return
		}
		now := w.clock.Now()
		event, err := w.repo.ClaimOutbox(ctx, w.owner, now, now.Add(w.lease))
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		if err != nil {
			w.logger.Error("claim outbox", "error", err)
			return
		}
		if err = w.publisher.Publish(ctx, event.EventType, append([]byte(nil), event.Payload...)); err == nil {
			if completeErr := w.repo.CompleteOutbox(ctx, event.ID, w.owner, w.clock.Now()); completeErr != nil {
				w.logger.Error("complete outbox", "event_id", event.ID, "error", completeErr)
			}
			continue
		}
		attemptsRemaining := w.maxAttempts - event.Attempts
		delay := backoff(event.Attempts)
		if failErr := w.repo.FailOutbox(ctx, event.ID, w.owner, err.Error(), attemptsRemaining, w.clock.Now().Add(delay)); failErr != nil {
			w.logger.Error("fail outbox", "event_id", event.ID, "error", failErr)
		} else {
			w.logger.Warn("outbox delivery failed", "event_id", event.ID, "attempts", event.Attempts, "retry_in", delay, "error", err)
		}
	}
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := math.Pow(2, float64(attempt-1))
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

type LogPublisher struct{ Logger *slog.Logger }

func (p LogPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Logger == nil {
		return fmt.Errorf("publisher logger is nil")
	}
	p.Logger.Info("outbox event", "event_type", eventType, "payload_bytes", len(payload))
	return nil
}
