package domain

import "testing"

func TestBatchStateMachineAllowsDocumentedTransitions(t *testing.T) {
	t.Parallel()
	allowed := map[BatchStatus][]BatchStatus{BatchRegistered: {BatchSampling}, BatchSampling: {BatchReleased, BatchRejected}, BatchReleased: {BatchExhausted}, BatchRejected: {BatchArchived}, BatchExhausted: {BatchArchived}}
	for from, targets := range allowed {
		for _, to := range targets {
			if !from.CanTransition(to) {
				t.Errorf("expected %s -> %s", from, to)
			}
		}
	}
}

func TestBatchStateMachineRejectsIllegalTransitions(t *testing.T) {
	t.Parallel()
	states := []BatchStatus{BatchRegistered, BatchSampling, BatchReleased, BatchRejected, BatchExhausted, BatchArchived}
	for _, from := range states {
		for _, to := range states {
			if from == to && from.CanTransition(to) {
				t.Errorf("self transition allowed for %s", from)
			}
		}
	}
	for _, transition := range [][2]BatchStatus{{BatchRegistered, BatchReleased}, {BatchReleased, BatchRegistered}, {BatchRejected, BatchReleased}, {BatchArchived, BatchSampling}, {BatchExhausted, BatchReleased}} {
		if transition[0].CanTransition(transition[1]) {
			t.Errorf("unexpected transition %s -> %s", transition[0], transition[1])
		}
	}
}

func TestOrderStateMachineAllowsWorkflow(t *testing.T) {
	t.Parallel()
	path := []OrderStatus{OrderDraft, OrderConfirmed, OrderAllocated, OrderInTransit, OrderDelivered, OrderSettled}
	for index := 0; index < len(path)-1; index++ {
		if !path[index].CanTransition(path[index+1]) {
			t.Fatalf("workflow rejects %s -> %s", path[index], path[index+1])
		}
	}
	for _, status := range []OrderStatus{OrderDraft, OrderConfirmed, OrderAllocated} {
		if !status.CanTransition(OrderCancelled) {
			t.Errorf("%s cannot cancel", status)
		}
	}
}

func TestOrderStateMachineRejectsSkippedAndTerminalTransitions(t *testing.T) {
	t.Parallel()
	transitions := [][2]OrderStatus{{OrderDraft, OrderAllocated}, {OrderConfirmed, OrderDelivered}, {OrderAllocated, OrderSettled}, {OrderDelivered, OrderCancelled}, {OrderCancelled, OrderDraft}, {OrderSettled, OrderDraft}}
	for _, transition := range transitions {
		if transition[0].CanTransition(transition[1]) {
			t.Errorf("unexpected transition %s -> %s", transition[0], transition[1])
		}
	}
}

func TestRolesAreExplicit(t *testing.T) {
	t.Parallel()
	for _, role := range []Role{RoleOperator, RoleFarmer, RoleInspector, RoleDispatcher, RoleFinance} {
		if !role.Valid() {
			t.Errorf("role %s invalid", role)
		}
	}
	for _, role := range []Role{"", "admin", "owner", "OPERATOR"} {
		if role.Valid() {
			t.Errorf("unknown role %s valid", role)
		}
	}
}
