package pagination

import (
	"errors"
	"strconv"
	"strings"
)

var ErrInvalidPage = errors.New("invalid page parameters")

type Page struct {
	Number int `json:"number"`
	Size   int `json:"size"`
}

func New(number, size int) (Page, error) {
	if number < 1 || size < 1 || size > 100 {
		return Page{}, ErrInvalidPage
	}
	return Page{Number: number, Size: size}, nil
}

func Default() Page {
	return Page{Number: 1, Size: 20}
}

func Parse(number, size string) (Page, error) {
	page := Default()
	var err error
	if strings.TrimSpace(number) != "" {
		page.Number, err = strconv.Atoi(number)
		if err != nil {
			return Page{}, ErrInvalidPage
		}
	}
	if strings.TrimSpace(size) != "" {
		page.Size, err = strconv.Atoi(size)
		if err != nil {
			return Page{}, ErrInvalidPage
		}
	}
	return New(page.Number, page.Size)
}

func (p Page) Offset() int {
	return (p.Number - 1) * p.Size
}

type Result[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

func Build[T any](items []T, total int, page Page) Result[T] {
	pages := 0
	if total > 0 {
		pages = (total + page.Size - 1) / page.Size
	}
	return Result[T]{
		Items:      items,
		Page:       page.Number,
		PageSize:   page.Size,
		TotalItems: total,
		TotalPages: pages,
	}
}
