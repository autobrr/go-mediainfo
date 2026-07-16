package mediainfo

import (
	"slices"
	"sort"
)

// canonicalStructuredFact retains one canonical scalar and its exact legacy
// compatibility projection until a stream snapshot is built.
type canonicalStructuredFact struct {
	name         fieldName
	canonical    string
	legacy       string
	retainLegacy bool
}

// canonicalStructuredFacts owns stable replacement, ordering, and projection
// of parser facts that need exact legacy compatibility values.
type canonicalStructuredFacts struct {
	values                   []canonicalStructuredFact
	legacySnapshotPresent    bool
	legacyRawSnapshotPresent bool
}

// Set replaces one staged fact.
func (f *canonicalStructuredFacts) Set(name fieldName, canonical, legacy string) {
	if f == nil || name == "" || canonical == "" || legacy == "" {
		return
	}
	fact := canonicalStructuredFact{name: name, canonical: canonical, legacy: legacy, retainLegacy: true}
	for index := range f.values {
		if f.values[index].name == name {
			f.values[index] = fact
			return
		}
	}
	f.values = append(f.values, fact)
}

// SetCanonical replaces one fact that needs no exported compatibility-map
// entry because its canonical projection is the sole source of truth.
func (f *canonicalStructuredFacts) SetCanonical(name fieldName, value string) {
	if f == nil || name == "" || value == "" {
		return
	}
	fact := canonicalStructuredFact{name: name, canonical: value}
	for index := range f.values {
		if f.values[index].name == name {
			f.values[index] = fact
			return
		}
	}
	f.values = append(f.values, fact)
}

// SetSame replaces one fact whose canonical and legacy values match.
func (f *canonicalStructuredFacts) SetSame(name fieldName, value string) {
	f.Set(name, value, value)
}

// Canonical returns the last canonical value staged for name.
func (f *canonicalStructuredFacts) Canonical(name fieldName) string {
	if f == nil {
		return ""
	}
	for index := range slices.Backward(f.values) {
		if f.values[index].name == name {
			return f.values[index].canonical
		}
	}
	return ""
}

// Legacy returns the last exact compatibility value staged for name.
func (f *canonicalStructuredFacts) Legacy(name fieldName) string {
	if f == nil {
		return ""
	}
	for index := range slices.Backward(f.values) {
		if f.values[index].name == name {
			return f.values[index].legacy
		}
	}
	return ""
}

// Delete removes one staged fact.
func (f *canonicalStructuredFacts) Delete(name fieldName) {
	if f == nil || name == "" {
		return
	}
	writeIndex := 0
	for _, fact := range f.values {
		if fact.name == name {
			continue
		}
		f.values[writeIndex] = fact
		writeIndex++
	}
	f.values = f.values[:writeIndex]
}

// PreserveLegacySnapshot records that the exported compatibility map existed
// even when the parser produced no scalar overrides.
func (f *canonicalStructuredFacts) PreserveLegacySnapshot() {
	if f != nil {
		f.legacySnapshotPresent = true
	}
}

// PreserveLegacyRawSnapshot records that the exported raw compatibility map
// existed even when the parser produced no compound overrides.
func (f *canonicalStructuredFacts) PreserveLegacyRawSnapshot() {
	if f != nil {
		f.legacyRawSnapshotPresent = true
	}
}

// Apply attaches exact compatibility values to canonical facts in registry
// order, creating a scalar only when the parser has not supplied one.
func (f *canonicalStructuredFacts) Apply(builder *canonicalStreamBuilder) {
	if f == nil || builder == nil {
		return
	}
	values := f.ordered(builder.store.stream(builder.ref).Kind)
	for _, fact := range values {
		if !builder.HasStructured(fact.name) {
			builder.DirectStructured(fact.name, fact.canonical)
		}
		if fact.retainLegacy {
			builder.MarkLegacyJSON(fact.name, fact.legacy)
		}
	}
}

// ApplyToStream merges staged facts into an existing canonical stream and
// publishes only the legacy compatibility snapshot derived from those facts.
func (f *canonicalStructuredFacts) ApplyToStream(stream *Stream) {
	if f == nil || stream == nil {
		return
	}
	values := f.ordered(stream.Kind)
	for _, fact := range values {
		replaceCanonicalSeedLegacyFill(stream, fact.name, fact.canonical, "", "")
		if fact.retainLegacy && fact.legacy != fact.canonical {
			setCanonicalSeedLegacyValue(stream, fact.name, fact.legacy, false)
		} else if !fact.retainLegacy {
			clearCanonicalSeedLegacyValue(stream, fact.name)
		}
		if spec, known := structuredFieldSpec(stream.Kind, string(fact.name)); known && spec.Measure == fieldMeasureMilliseconds {
			if decimals := decimalFractionDigits(fact.legacy); decimals > 3 {
				setCanonicalSeedStructuredDecimals(stream, fact.name, uint8(decimals))
			}
		}
	}
	refreshCanonicalLegacyMaps(stream)
	if f.legacySnapshotPresent && stream.JSON == nil {
		stream.JSON = map[string]string{}
	}
	if f.legacyRawSnapshotPresent && stream.JSONRaw == nil {
		stream.JSONRaw = map[string]string{}
	}
}

// ordered returns a stable registry-ordered copy of the staged facts.
func (f *canonicalStructuredFacts) ordered(kind StreamKind) []canonicalStructuredFact {
	if f == nil {
		return nil
	}
	values := append([]canonicalStructuredFact(nil), f.values...)
	sort.SliceStable(values, func(left, right int) bool {
		leftOrder := structuredFieldOrder(kind, string(values[left].name))
		rightOrder := structuredFieldOrder(kind, string(values[right].name))
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return values[left].name < values[right].name
	})
	return values
}
