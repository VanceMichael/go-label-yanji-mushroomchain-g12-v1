package money

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestParseAcceptedFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  Amount
	}{
		{name: "whole", input: "12", want: 1200},
		{name: "one decimal", input: "12.3", want: 1230},
		{name: "two decimals", input: "12.34", want: 1234},
		{name: "zero", input: "0", want: 0},
		{name: "trimmed", input: " 9.05 ", want: 905},
		{name: "large", input: "9000000.99", want: 900000099},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(test.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("Parse(%q)=%d want %d", test.input, got, test.want)
			}
		})
	}
}

func TestParseRejectsMalformedValues(t *testing.T) {
	t.Parallel()
	values := []string{"", " ", "-1", "-0.01", ".50", "1.", "1.234", "one", "1,000", "++1", "1..2"}
	for _, value := range values {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(value)
			if err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", value)
			}
			if strings.HasPrefix(value, "-") && !errors.Is(err, ErrNegative) {
				t.Fatalf("Parse(%q) error=%v want negative", value, err)
			}
		})
	}
}

func TestAmountStringRoundTrip(t *testing.T) {
	t.Parallel()
	for _, value := range []Amount{0, 1, 9, 10, 99, 100, 101, 999, 123456789} {
		encoded := value.String()
		parsed, err := Parse(encoded)
		if err != nil {
			t.Fatalf("parse %q: %v", encoded, err)
		}
		if parsed != value {
			t.Fatalf("round trip got %d want %d", parsed, value)
		}
	}
}

func TestCentsRejectsNegative(t *testing.T) {
	t.Parallel()
	if _, err := Cents(-1); !errors.Is(err, ErrNegative) {
		t.Fatalf("Cents error=%v", err)
	}
	if amount, err := Cents(1); err != nil || amount != 1 {
		t.Fatalf("Cents(1)=%d,%v", amount, err)
	}
}

func TestAddAndMultiply(t *testing.T) {
	t.Parallel()
	sum, err := Amount(125).Add(375)
	if err != nil || sum != 500 {
		t.Fatalf("Add=%d,%v", sum, err)
	}
	product, err := Amount(125).Multiply(4)
	if err != nil || product != 500 {
		t.Fatalf("Multiply=%d,%v", product, err)
	}
	if _, err = Amount(1).Multiply(-1); !errors.Is(err, ErrNegative) {
		t.Fatalf("negative multiplication error=%v", err)
	}
}

func TestOverflowDetection(t *testing.T) {
	t.Parallel()
	maximum := Amount(math.MaxInt64)
	if _, err := maximum.Add(1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Add overflow error=%v", err)
	}
	if _, err := Amount(math.MaxInt64/2 + 1).Multiply(2); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Multiply overflow error=%v", err)
	}
	if _, err := Parse("92233720368547759.00"); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Parse overflow error=%v", err)
	}
}

func TestMustCentsPanicsForNegative(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("MustCents did not panic")
		}
	}()
	_ = MustCents(-10)
}
