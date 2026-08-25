package clock

import (
	"sync"
	"testing"
	"time"
)

func TestFixedAdvanceAndSet(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 25, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))
	fixed := NewFixed(start)
	if !fixed.Now().Equal(start) {
		t.Fatalf("now=%s", fixed.Now())
	}
	fixed.Advance(90 * time.Minute)
	if !fixed.Now().Equal(start.Add(90 * time.Minute)) {
		t.Fatalf("advanced=%s", fixed.Now())
	}
	target := start.Add(24 * time.Hour)
	fixed.Set(target)
	if !fixed.Now().Equal(target) {
		t.Fatalf("set=%s", fixed.Now())
	}
	if fixed.Now().Location() != time.UTC {
		t.Fatalf("location=%s", fixed.Now().Location())
	}
}

func TestFixedSupportsConcurrentReadersAndWriter(t *testing.T) {
	start := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	fixed := NewFixed(start)
	var wg sync.WaitGroup
	for index := 0; index < 20; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for count := 0; count < 100; count++ {
				_ = fixed.Now()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for count := 0; count < 100; count++ {
			fixed.Advance(time.Millisecond)
		}
	}()
	wg.Wait()
	if fixed.Now() != start.Add(100*time.Millisecond) {
		t.Fatalf("now=%s", fixed.Now())
	}
}

func TestSystemReturnsUTC(t *testing.T) {
	t.Parallel()
	if (System{}).Now().Location() != time.UTC {
		t.Fatalf("location=%s", (System{}).Now().Location())
	}
}
