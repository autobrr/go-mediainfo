package mediainfo

const (
	embeddedAssetMaxNameBytes     = int64(4 << 10)
	embeddedAssetMaxDescription   = int64(4 << 10)
	embeddedAssetMaxMIMEBytes     = int64(1 << 10)
	embeddedAssetMaxStringBytes   = int64(1 << 20)
	embeddedAssetMaxItems         = int64(256)
	embeddedAssetMaxPayloadBytes  = int64(4 << 20)
	embeddedAssetMaxImageProbe    = int64(64 << 10)
	embeddedAssetMaxRetainedBytes = int64(32 << 20)
)

// embeddedAssetRejection identifies why untrusted embedded metadata could not
// reserve a bounded parser resource. Values never include attacker-controlled
// content and are suitable for stable internal diagnostics.
type embeddedAssetRejection string

const (
	embeddedAssetAccepted       embeddedAssetRejection = ""
	embeddedAssetInvalidRange   embeddedAssetRejection = "invalid_range"
	embeddedAssetItemLimit      embeddedAssetRejection = "item_limit"
	embeddedAssetStringLimit    embeddedAssetRejection = "string_limit"
	embeddedAssetPayloadLimit   embeddedAssetRejection = "payload_limit"
	embeddedAssetAggregateLimit embeddedAssetRejection = "aggregate_limit"
)

// embeddedAssetBudget bounds retained attachment and cover-art resources for
// one file analysis. It is intentionally not concurrency-safe; parsers consume
// it synchronously while walking one input.
type embeddedAssetBudget struct {
	items         int64
	stringBytes   int64
	retainedBytes int64
}

// checkedEmbeddedRange returns the contained end offset for an untrusted size.
// It rejects unknown, overflowing, negative, and parent-escaping ranges before
// narrowing the unsigned size to int64.
func checkedEmbeddedRange(start int64, size uint64, limit int64) (int64, embeddedAssetRejection) {
	if size == unknownVintSize || start < 0 || limit < start {
		return 0, embeddedAssetInvalidRange
	}
	remaining := limit - start
	if size > uint64(remaining) {
		return 0, embeddedAssetInvalidRange
	}
	return start + int64(size), embeddedAssetAccepted
}

// checkedEmbeddedOffset resolves an unsigned relative offset inside limit
// without allowing signed conversion or addition to wrap.
func checkedEmbeddedOffset(base int64, relative uint64, limit int64) (int64, embeddedAssetRejection) {
	end, reason := checkedEmbeddedRange(base, relative, limit)
	if reason != embeddedAssetAccepted || end >= limit {
		return 0, embeddedAssetInvalidRange
	}
	return end, embeddedAssetAccepted
}

// allowAllocation validates a single transient allocation against a hard cap.
// It does not consume retained-byte budget because the allocation is released
// before the parser returns its retained metadata.
func (b *embeddedAssetBudget) allowAllocation(size uint64, maximum int64) embeddedAssetRejection {
	if b == nil || maximum < 0 || size > uint64(maximum) {
		return embeddedAssetPayloadLimit
	}
	return embeddedAssetAccepted
}

// reserveItem consumes one embedded-asset slot when capacity remains.
func (b *embeddedAssetBudget) reserveItem() embeddedAssetRejection {
	if b == nil || b.items >= embeddedAssetMaxItems {
		return embeddedAssetItemLimit
	}
	b.items++
	return embeddedAssetAccepted
}

// reserveString consumes string capacity after enforcing a per-value maximum.
// A failed reservation leaves all counters unchanged.
func (b *embeddedAssetBudget) reserveString(size uint64, maximum int64) embeddedAssetRejection {
	if b == nil || maximum < 0 || size > uint64(maximum) {
		return embeddedAssetStringLimit
	}
	remaining := embeddedAssetMaxStringBytes - b.stringBytes
	if remaining < 0 || size > uint64(remaining) {
		return embeddedAssetAggregateLimit
	}
	b.stringBytes += int64(size)
	return embeddedAssetAccepted
}

// reservePayload consumes retained-payload capacity after enforcing a per-value
// maximum. A failed reservation leaves all counters unchanged.
func (b *embeddedAssetBudget) reservePayload(size uint64, maximum int64) embeddedAssetRejection {
	if b == nil || maximum < 0 || size > uint64(maximum) {
		return embeddedAssetPayloadLimit
	}
	remaining := embeddedAssetMaxRetainedBytes - b.retainedBytes
	if remaining < 0 || size > uint64(remaining) {
		return embeddedAssetAggregateLimit
	}
	b.retainedBytes += int64(size)
	return embeddedAssetAccepted
}

// releasePayload rolls back a successful payload reservation when the caller
// cannot retain the payload. Invalid releases leave the counter unchanged.
func (b *embeddedAssetBudget) releasePayload(size uint64) {
	if b == nil || b.retainedBytes < 0 || size > uint64(b.retainedBytes) {
		return
	}
	b.retainedBytes -= int64(size)
}
