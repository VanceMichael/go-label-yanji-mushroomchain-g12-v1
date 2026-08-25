package service

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/clock"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/pagination"
	"github.com/VanceMichael/go-label-yanji-mushroomchain-g12-v1/internal/testkit"
)

func TestCancelledBatchInspectionDoesNotCommit(t *testing.T) {
	data := testkit.New(t)
	batches := NewBatches(data.Store, data.Store, clock.NewFixed(data.Now))
	inspector := data.Users[domain.RoleInspector]
	identity := Identity{UserID: inspector.ID, TenantID: data.TenantID, Role: domain.RoleInspector, SessionID: "inspection-session"}

	cancelledBatch := data.Batch(domain.BatchRegistered, 100)
	cancelledBatch.ID = "batch-cancelled-inspection"
	if err := data.Store.CreateBatch(context.Background(), cancelledBatch); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(WithIdentity(context.Background(), identity))
	cancel()
	_, err := batches.Inspect(request, InspectBatchInput{
		BatchID: cancelledBatch.ID, Decision: domain.InspectionApproved, MoistureBP: 6100,
		SampleCount: 10, Notes: "request cancelled", RequestID: "cancelled-inspection",
		ExpectedVersion: 1,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inspection error=%v", err)
	}
	stored, err := data.Store.GetBatch(context.Background(), data.TenantID, cancelledBatch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.BatchRegistered || stored.Version != 1 {
		t.Fatalf("cancelled request changed batch=%+v", stored)
	}
	inspections, err := data.Store.ListInspections(context.Background(), data.TenantID, cancelledBatch.ID)
	if err != nil || len(inspections) != 0 {
		t.Fatalf("cancelled request left inspections=%+v err=%v", inspections, err)
	}
	audits, total, err := data.Store.ListAudit(context.Background(), data.TenantID, "batch", cancelledBatch.ID, pagination.Page{Number: 1, Size: 20})
	if err != nil || total != 0 || len(audits) != 0 {
		t.Fatalf("cancelled request left audits=%+v total=%d err=%v", audits, total, err)
	}

	successBatch := data.Batch(domain.BatchRegistered, 100)
	successBatch.ID = "batch-successful-inspection"
	if err := data.Store.CreateBatch(context.Background(), successBatch); err != nil {
		t.Fatal(err)
	}
	released, err := batches.Inspect(WithIdentity(context.Background(), identity), InspectBatchInput{
		BatchID: successBatch.ID, Decision: domain.InspectionApproved, MoistureBP: 6100,
		SampleCount: 10, Notes: "normal inspection", RequestID: "successful-inspection",
		ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != domain.BatchReleased || released.Version != 3 {
		t.Fatalf("normal inspection result=%+v", released)
	}
}
