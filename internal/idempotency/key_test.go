package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestDigestIsStable(t *testing.T) {
	t.Parallel()
	scope := Scope{TenantID: "tenant-a", Method: "post", Path: "/v1/orders", Key: "request-1"}
	first, err := scope.Digest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := scope.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("digest changed: %s != %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("digest length=%d", len(first))
	}
}

func TestDigestCanonicalizesMethodAndWhitespace(t *testing.T) {
	t.Parallel()
	a, _ := Scope{TenantID: "tenant-a", Method: " post ", Path: "/v1/orders", Key: " request-1 "}.Digest()
	b, _ := Scope{TenantID: "tenant-a", Method: "POST", Path: "/v1/orders", Key: "request-1"}.Digest()
	if a != b {
		t.Fatalf("canonical digest mismatch %s %s", a, b)
	}
}

func TestDigestSeparatesSecurityScope(t *testing.T) {
	t.Parallel()
	base := Scope{TenantID: "tenant-a", Method: "POST", Path: "/v1/orders", Key: "request-1"}
	first, _ := base.Digest()
	variants := []Scope{{TenantID: "tenant-b", Method: base.Method, Path: base.Path, Key: base.Key}, {TenantID: base.TenantID, Method: "PUT", Path: base.Path, Key: base.Key}, {TenantID: base.TenantID, Method: base.Method, Path: "/v1/batches", Key: base.Key}, {TenantID: base.TenantID, Method: base.Method, Path: base.Path, Key: "request-2"}}
	for _, variant := range variants {
		got, err := variant.Digest()
		if err != nil {
			t.Fatal(err)
		}
		if got == first {
			t.Fatalf("variant did not change digest: %+v", variant)
		}
	}
}

func TestValidateRejectsMissingParts(t *testing.T) {
	t.Parallel()
	tests := []Scope{{}, {TenantID: "tenant", Method: "POST", Path: "/path"}, {TenantID: "tenant", Method: "POST", Key: "key"}, {TenantID: "tenant", Path: "/path", Key: "key"}, {Method: "POST", Path: "/path", Key: "key"}, {TenantID: "tenant", Method: "POST", Path: "/path", Key: strings.Repeat("x", 129)}}
	for _, test := range tests {
		if err := test.Validate(); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("Validate(%+v) error=%v", test, err)
		}
		if _, err := test.Digest(); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("Digest(%+v) error=%v", test, err)
		}
	}
}
