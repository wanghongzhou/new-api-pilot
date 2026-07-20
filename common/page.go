package common

type PageData[T any] struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
	Items    []T   `json:"items"`
}

func NewPageData[T any](page, pageSize int, total int64, items []T) PageData[T] {
	if items == nil {
		items = make([]T, 0)
	}
	return PageData[T]{Page: page, PageSize: pageSize, Total: total, Items: items}
}
