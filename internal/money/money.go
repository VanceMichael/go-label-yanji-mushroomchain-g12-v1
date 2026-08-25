package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrNegative = errors.New("money cannot be negative")
	ErrFormat   = errors.New("invalid money format")
	ErrOverflow = errors.New("money overflow")
)

type Amount int64

func Cents(value int64) (Amount, error) {
	if value < 0 {
		return 0, ErrNegative
	}
	return Amount(value), nil
}

func MustCents(value int64) Amount {
	amount, err := Cents(value)
	if err != nil {
		panic(err)
	}
	return amount
}

func Parse(value string) (Amount, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") {
		if strings.HasPrefix(value, "-") {
			return 0, ErrNegative
		}
		return 0, ErrFormat
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrFormat
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	fraction := int64(0)
	if len(parts) == 2 {
		if len(parts[1]) == 1 {
			parts[1] += "0"
		}
		if len(parts[1]) != 2 {
			return 0, ErrFormat
		}
		fraction, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrFormat, err)
		}
	}
	if whole > (int64(^uint64(0)>>1)-fraction)/100 {
		return 0, ErrOverflow
	}
	return Amount(whole*100 + fraction), nil
}

func (a Amount) Cents() int64 {
	return int64(a)
}

func (a Amount) String() string {
	return fmt.Sprintf("%d.%02d", int64(a)/100, int64(a)%100)
}

func (a Amount) Add(other Amount) (Amount, error) {
	if other > 0 && a > Amount(int64(^uint64(0)>>1))-other {
		return 0, ErrOverflow
	}
	return a + other, nil
}

func (a Amount) Multiply(quantity int64) (Amount, error) {
	if quantity < 0 {
		return 0, ErrNegative
	}
	if quantity != 0 && a > Amount(int64(^uint64(0)>>1)/quantity) {
		return 0, ErrOverflow
	}
	return a * Amount(quantity), nil
}
