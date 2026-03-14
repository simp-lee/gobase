package domain

import (
	"encoding/json"
	"testing"
)

func TestPageResult_ZeroValue(t *testing.T) {
	var pr PageResult[string]
	if pr.Items != nil {
		t.Errorf("expected nil Items, got %v", pr.Items)
	}
	if pr.TotalItems != 0 || pr.TotalPages != 0 || pr.CurrentPage != 0 || pr.PageSize != 0 {
		t.Error("expected zero values for pagination fields")
	}
}

func TestPageResult_WithItems(t *testing.T) {
	pr := PageResult[int]{
		Items:       []int{1, 2, 3},
		TotalItems:  10,
		TotalPages:  4,
		CurrentPage: 1,
		PageSize:    3,
	}
	if len(pr.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(pr.Items))
	}
	if pr.TotalItems != 10 {
		t.Errorf("expected TotalItems=10, got %d", pr.TotalItems)
	}
	if pr.TotalPages != 4 {
		t.Errorf("expected TotalPages=4, got %d", pr.TotalPages)
	}
}

func TestPageResult_JSON(t *testing.T) {
	pr := PageResult[string]{
		Items:       []string{"a", "b"},
		TotalItems:  5,
		TotalPages:  3,
		CurrentPage: 1,
		PageSize:    2,
	}
	data, err := json.Marshal(pr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got PageResult[string]
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.TotalItems != pr.TotalItems || got.CurrentPage != pr.CurrentPage || got.PageSize != pr.PageSize || got.TotalPages != pr.TotalPages {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, pr)
	}
	if len(got.Items) != 2 || got.Items[0] != "a" || got.Items[1] != "b" {
		t.Errorf("items mismatch: got %v", got.Items)
	}
}

func TestPageResult_StructItems(t *testing.T) {
	type item struct {
		Name string `json:"name"`
	}
	pr := PageResult[item]{
		Items:       []item{{Name: "foo"}, {Name: "bar"}},
		TotalItems:  2,
		TotalPages:  1,
		CurrentPage: 1,
		PageSize:    10,
	}
	if pr.Items[0].Name != "foo" {
		t.Errorf("expected first item name=foo, got %s", pr.Items[0].Name)
	}
}
