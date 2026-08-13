package common

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type JSONInt64 int64

func (value JSONInt64) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(strconv.FormatInt(int64(value), 10))), nil
}

type PageData[T any] struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"-"`
	Items    []T   `json:"items"`
}

func NewPageData[T any](page, pageSize int, total int64, items []T) PageData[T] {
	if items == nil {
		items = make([]T, 0)
	}
	return PageData[T]{Page: page, PageSize: pageSize, Total: total, Items: items}
}

func (data PageData[T]) MarshalJSON() ([]byte, error) {
	type wirePageData struct {
		Page     int       `json:"page"`
		PageSize int       `json:"page_size"`
		Total    JSONInt64 `json:"total"`
		Items    []T       `json:"items"`
	}
	return json.Marshal(wirePageData{Page: data.Page, PageSize: data.PageSize, Total: JSONInt64(data.Total), Items: data.Items})
}

func (data *PageData[T]) UnmarshalJSON(raw []byte) error {
	type wirePageData struct {
		Page     int             `json:"page"`
		PageSize int             `json:"page_size"`
		Total    json.RawMessage `json:"total"`
		Items    []T             `json:"items"`
	}
	var wire wirePageData
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	var totalText string
	if err := json.Unmarshal(wire.Total, &totalText); err != nil {
		var legacy int64
		if numberErr := json.Unmarshal(wire.Total, &legacy); numberErr != nil {
			return err
		}
		totalText = strconv.FormatInt(legacy, 10)
	}
	total, err := strconv.ParseInt(totalText, 10, 64)
	if err != nil || total < 0 {
		return fmt.Errorf("invalid page total %q", totalText)
	}
	data.Page, data.PageSize, data.Total, data.Items = wire.Page, wire.PageSize, total, wire.Items
	if data.Items == nil {
		data.Items = make([]T, 0)
	}
	return nil
}
