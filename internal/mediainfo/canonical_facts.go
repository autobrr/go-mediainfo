package mediainfo

import (
	"slices"
	"sort"
)

// canonicalStructuredFact retains one canonical scalar and optional projection
// precision until a stream snapshot is built.
type canonicalStructuredFact struct {
	name       fieldName
	canonical  string
	projection string
}

// canonicalStructuredFacts owns stable replacement, ordering, and projection
// of parser facts.
type canonicalStructuredFacts struct {
	values []canonicalStructuredFact
}

// Set replaces one staged fact.
func (f *canonicalStructuredFacts) Set(name fieldName, canonical, projection string) {
	if f == nil || name == "" || canonical == "" || projection == "" {
		return
	}
	fact := canonicalStructuredFact{name: name, canonical: canonical, projection: projection}
	for index := range f.values {
		if f.values[index].name == name {
			f.values[index] = fact
			return
		}
	}
	f.values = append(f.values, fact)
}

// SetCanonical replaces one fact whose registry projection is sufficient.
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

// SetSame replaces one fact whose canonical and projected values match.
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

// Projection returns the last explicit projected value staged for name.
func (f *canonicalStructuredFacts) Projection(name fieldName) string {
	if f == nil {
		return ""
	}
	for index := range slices.Backward(f.values) {
		if f.values[index].name == name {
			return f.values[index].projection
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

// Apply attaches canonical facts in registry order.
func (f *canonicalStructuredFacts) Apply(builder *canonicalStreamBuilder) {
	if f == nil || builder == nil {
		return
	}
	values := f.ordered(builder.store.stream(builder.ref).Kind)
	for _, fact := range values {
		if !builder.HasStructured(fact.name) {
			builder.DirectStructured(fact.name, fact.canonical)
		}
		if spec, known := structuredFieldSpec(builder.store.stream(builder.ref).Kind, string(fact.name)); known && spec.Measure == fieldMeasureMilliseconds {
			if decimals := decimalFractionDigits(fact.projection); decimals > 3 {
				builder.SetStructuredDecimals(fact.name, uint8(decimals))
			}
		}
	}
}

// ApplyToStream merges staged facts into an existing canonical stream.
func (f *canonicalStructuredFacts) ApplyToStream(stream *Stream) {
	if f == nil || stream == nil {
		return
	}
	values := f.ordered(stream.Kind)
	for _, fact := range values {
		replaceCanonicalSeedFill(stream, fact.name, fact.canonical, "", "")
		if spec, known := structuredFieldSpec(stream.Kind, string(fact.name)); known && spec.Measure == fieldMeasureMilliseconds {
			if decimals := decimalFractionDigits(fact.projection); decimals > 3 {
				setCanonicalSeedStructuredDecimals(stream, fact.name, uint8(decimals))
			}
		}
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
