package mediainfo

import (
	"math"
	"testing"
)

func TestCheckedEmbeddedRange(t *testing.T) {
	tests := []struct {
		name  string
		start int64
		size  uint64
		limit int64
		end   int64
		ok    bool
	}{
		{name: "empty", start: 5, size: 0, limit: 10, end: 5, ok: true},
		{name: "exact", start: 5, size: 5, limit: 10, end: 10, ok: true},
		{name: "escaping", start: 5, size: 6, limit: 10},
		{name: "unknown", start: 5, size: unknownVintSize, limit: 10},
		{name: "negative start", start: -1, size: 1, limit: 10},
		{name: "reversed", start: 11, size: 0, limit: 10},
		{name: "unsigned wrap", start: 1, size: math.MaxUint64 - 1, limit: math.MaxInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end, reason := checkedEmbeddedRange(tt.start, tt.size, tt.limit)
			if gotOK := reason == embeddedAssetAccepted; gotOK != tt.ok {
				t.Fatalf("reason = %q, want success %v", reason, tt.ok)
			}
			if tt.ok && end != tt.end {
				t.Fatalf("end = %d, want %d", end, tt.end)
			}
		})
	}
}

func TestCheckedEmbeddedOffset(t *testing.T) {
	if got, reason := checkedEmbeddedOffset(100, 20, 200); reason != embeddedAssetAccepted || got != 120 {
		t.Fatalf("valid offset = %d, %q", got, reason)
	}
	for _, relative := range []uint64{100, math.MaxUint64} {
		if _, reason := checkedEmbeddedOffset(100, relative, 200); reason != embeddedAssetInvalidRange {
			t.Fatalf("relative %d reason = %q", relative, reason)
		}
	}
}

func TestEmbeddedAssetBudgetLimitsAreTransactional(t *testing.T) {
	budget := &embeddedAssetBudget{}
	for i := range embeddedAssetMaxItems {
		if reason := budget.reserveItem(); reason != embeddedAssetAccepted {
			t.Fatalf("item %d rejected: %q", i, reason)
		}
	}
	if reason := budget.reserveItem(); reason != embeddedAssetItemLimit || budget.items != embeddedAssetMaxItems {
		t.Fatalf("over-limit item = %q, items=%d", reason, budget.items)
	}

	if reason := budget.reserveString(uint64(embeddedAssetMaxNameBytes+1), embeddedAssetMaxNameBytes); reason != embeddedAssetStringLimit || budget.stringBytes != 0 {
		t.Fatalf("over-limit string = %q, bytes=%d", reason, budget.stringBytes)
	}
	if reason := budget.reserveString(uint64(embeddedAssetMaxNameBytes), embeddedAssetMaxNameBytes); reason != embeddedAssetAccepted {
		t.Fatalf("exact string rejected: %q", reason)
	}
	budget.stringBytes = embeddedAssetMaxStringBytes
	if reason := budget.reserveString(1, embeddedAssetMaxNameBytes); reason != embeddedAssetAggregateLimit || budget.stringBytes != embeddedAssetMaxStringBytes {
		t.Fatalf("aggregate string = %q, bytes=%d", reason, budget.stringBytes)
	}

	if reason := budget.reservePayload(uint64(embeddedAssetMaxPayloadBytes+1), embeddedAssetMaxPayloadBytes); reason != embeddedAssetPayloadLimit || budget.retainedBytes != 0 {
		t.Fatalf("over-limit payload = %q, bytes=%d", reason, budget.retainedBytes)
	}
	if reason := budget.reservePayload(uint64(embeddedAssetMaxPayloadBytes), embeddedAssetMaxPayloadBytes); reason != embeddedAssetAccepted {
		t.Fatalf("exact payload rejected: %q", reason)
	}
	budget.retainedBytes = embeddedAssetMaxRetainedBytes
	if reason := budget.reservePayload(1, embeddedAssetMaxPayloadBytes); reason != embeddedAssetAggregateLimit || budget.retainedBytes != embeddedAssetMaxRetainedBytes {
		t.Fatalf("aggregate payload = %q, bytes=%d", reason, budget.retainedBytes)
	}
}
