package pagination

import (
	"errors"
	"testing"
)

func TestNewValidPages(t *testing.T) {
	t.Parallel()
	tests := []struct{ number, size, offset int }{{1, 1, 0}, {1, 20, 0}, {2, 20, 20}, {5, 100, 400}}
	for _, test := range tests {
		page, err := New(test.number, test.size)
		if err != nil {
			t.Fatalf("New(%d,%d): %v", test.number, test.size, err)
		}
		if page.Offset() != test.offset {
			t.Fatalf("offset=%d want %d", page.Offset(), test.offset)
		}
	}
}

func TestNewRejectsInvalidPages(t *testing.T) {
	t.Parallel()
	tests := []struct{ number, size int }{{0, 20}, {-1, 20}, {1, 0}, {1, -1}, {1, 101}, {0, 0}}
	for _, test := range tests {
		if _, err := New(test.number, test.size); !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("New(%d,%d) error=%v", test.number, test.size, err)
		}
	}
}

func TestParseDefaultsAndOverrides(t *testing.T) {
	t.Parallel()
	page, err := Parse("", "")
	if err != nil {
		t.Fatal(err)
	}
	if page != Default() {
		t.Fatalf("default=%+v", page)
	}
	page, err = Parse("3", "40")
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 3 || page.Size != 40 || page.Offset() != 80 {
		t.Fatalf("parsed=%+v", page)
	}
}

func TestParseRejectsNonNumbers(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{{"x", "20"}, {"1", "many"}, {"1.5", "20"}, {"2", "101"}} {
		if _, err := Parse(pair[0], pair[1]); !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("Parse(%q,%q) error=%v", pair[0], pair[1], err)
		}
	}
}

func TestBuildCalculatesTotals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		total, size, wantPages int
	}{{"empty", 0, 20, 0}, {"partial", 1, 20, 1}, {"full", 20, 20, 1}, {"next", 21, 20, 2}, {"many", 101, 25, 5}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Build([]string{"item"}, test.total, Page{Number: 2, Size: test.size})
			if result.TotalPages != test.wantPages {
				t.Fatalf("pages=%d want %d", result.TotalPages, test.wantPages)
			}
			if result.Page != 2 || result.PageSize != test.size || result.TotalItems != test.total {
				t.Fatalf("metadata=%+v", result)
			}
			if len(result.Items) != 1 {
				t.Fatalf("items=%v", result.Items)
			}
		})
	}
}
