package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestObjectMetaJSON(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	meta := ObjectMeta{
		ID:        "123",
		Name:      "test",
		CreatedAt: &now,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ObjectMeta
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "123" || got.Name != "test" || got.CreatedAt == nil {
		t.Errorf("unexpected: %+v", got)
	}
}
