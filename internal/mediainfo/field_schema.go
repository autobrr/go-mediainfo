package mediainfo

import "strings"

// fieldName is a canonical field identifier used by the store and structured outputs.
type fieldName string

// fieldValue retains the parser-provided canonical text without float conversion.
type fieldValue struct {
	Text string
}

// fieldValueType controls how a field is represented by structured projections.
type fieldValueType uint8

// field value types distinguish ordinary strings, numeric scalars, and prebuilt nodes.
const (
	fieldValueString fieldValueType = iota
	fieldValueInteger
	fieldValueDecimal
	fieldValueNode
)

// fieldMeasure identifies the base unit used to derive display strings.
type fieldMeasure uint8

// field measures describe the canonical units accepted by finalization.
const (
	fieldMeasureNone fieldMeasure = iota
	fieldMeasureMilliseconds
	fieldMeasureBytes
	fieldMeasureBitsPerSecond
	fieldMeasureHertz
	fieldMeasureChannels
	fieldMeasurePixels
	fieldMeasureBits
)

// fieldOptions selects a field's text and structured projection visibility.
type fieldOptions struct {
	ShowText       bool
	ShowStructured bool
	ShowXML        bool
	ValueType      fieldValueType
}

// fieldSpec defines a known field's names, ordering, units, and projection policy.
type fieldSpec struct {
	Name          fieldName
	Measure       fieldMeasure
	Options       fieldOptions
	StructuredKey string
	Order         int
	TextLabel     string
	StringSibling fieldName
}

// canonicalFieldDefinitions contains the shared definitions for canonical fields.
var canonicalFieldDefinitions = map[fieldName]fieldSpec{
	"Format":                     canonicalField("Format", "Format", fieldMeasureNone, fieldValueString),
	"Format_Profile":             canonicalField("Format_Profile", "Format profile", fieldMeasureNone, fieldValueString),
	"Format_Level":               canonicalField("Format_Level", "Format level", fieldMeasureNone, fieldValueString),
	"Format_Tier":                canonicalField("Format_Tier", "Format tier", fieldMeasureNone, fieldValueString),
	"CodecID":                    canonicalField("CodecID", "Codec ID", fieldMeasureNone, fieldValueString),
	"Format_Settings_Endianness": canonicalField("Format_Settings_Endianness", "Format settings, Endianness", fieldMeasureNone, fieldValueString),
	"Format_Settings_Sign":       canonicalField("Format_Settings_Sign", "Format settings, Sign", fieldMeasureNone, fieldValueString),
	"Duration":                   measuredField("Duration", "Duration", fieldMeasureMilliseconds, fieldValueDecimal),
	"Source_Duration":            measuredField("Source_Duration", "Source duration", fieldMeasureMilliseconds, fieldValueDecimal),
	"Source_Duration_LastFrame":  measuredField("Source_Duration_LastFrame", "Source_Duration_LastFrame", fieldMeasureMilliseconds, fieldValueDecimal),
	"BitRate":                    measuredField("BitRate", "Bit rate", fieldMeasureBitsPerSecond, fieldValueInteger),
	"BitRate_Nominal":            measuredField("BitRate_Nominal", "Nominal bit rate", fieldMeasureBitsPerSecond, fieldValueInteger),
	"BitRate_Maximum":            measuredField("BitRate_Maximum", "Maximum bit rate", fieldMeasureBitsPerSecond, fieldValueInteger),
	"OverallBitRate":             measuredField("OverallBitRate", "Overall bit rate", fieldMeasureBitsPerSecond, fieldValueInteger),
	"FileSize":                   measuredField("FileSize", "File size", fieldMeasureBytes, fieldValueInteger),
	"StreamSize":                 measuredField("StreamSize", "Stream size", fieldMeasureBytes, fieldValueInteger),
	"Source_StreamSize":          measuredField("Source_StreamSize", "Source stream size", fieldMeasureBytes, fieldValueInteger),
	"Width":                      measuredField("Width", "Width", fieldMeasurePixels, fieldValueInteger),
	"Height":                     measuredField("Height", "Height", fieldMeasurePixels, fieldValueInteger),
	"Channels":                   measuredField("Channels", "Channel(s)", fieldMeasureChannels, fieldValueInteger),
	"SamplingRate":               measuredField("SamplingRate", "Sampling rate", fieldMeasureHertz, fieldValueInteger),
	"BitDepth":                   measuredField("BitDepth", "Bit depth", fieldMeasureBits, fieldValueInteger),
	"FrameRate":                  canonicalField("FrameRate", "Frame rate", fieldMeasureNone, fieldValueDecimal),
	"FrameRate_Mode":             canonicalField("FrameRate_Mode", "Frame rate mode", fieldMeasureNone, fieldValueString),
	"BitRate_Mode":               canonicalField("BitRate_Mode", "Bit rate mode", fieldMeasureNone, fieldValueString),
	"OverallBitRate_Mode":        canonicalField("OverallBitRate_Mode", "Overall bit rate mode", fieldMeasureNone, fieldValueString),
	"DisplayAspectRatio":         canonicalField("DisplayAspectRatio", "Display aspect ratio", fieldMeasureNone, fieldValueDecimal),
	"ChannelLayout":              canonicalField("ChannelLayout", "Channel layout", fieldMeasureNone, fieldValueString),
	"Compression_Mode":           canonicalField("Compression_Mode", "Compression mode", fieldMeasureNone, fieldValueString),
	"Language":                   canonicalField("Language", "Language", fieldMeasureNone, fieldValueString),
	"Title":                      canonicalField("Title", "Title", fieldMeasureNone, fieldValueString),
	"Encoded_Application":        canonicalField("Encoded_Application", "Writing application", fieldMeasureNone, fieldValueString),
	"Encoded_Library":            canonicalField("Encoded_Library", "Writing library", fieldMeasureNone, fieldValueString),
	"Encoded_Library_Settings":   canonicalField("Encoded_Library_Settings", "Encoding settings", fieldMeasureNone, fieldValueString),
}

// textLabelCanonicalNames resolves legacy display labels to known canonical fields.
var textLabelCanonicalNames = func() map[string]fieldName {
	result := make(map[string]fieldName, len(canonicalFieldDefinitions))
	for name, spec := range canonicalFieldDefinitions {
		result[spec.TextLabel] = name
	}
	return result
}()

// canonicalField creates a known field visible in both text and structured projections.
func canonicalField(name fieldName, textLabel string, measure fieldMeasure, valueType fieldValueType) fieldSpec {
	return fieldSpec{
		Name:          name,
		Measure:       measure,
		Options:       fieldOptions{ShowText: true, ShowStructured: true, ShowXML: true, ValueType: valueType},
		StructuredKey: string(name),
		TextLabel:     textLabel,
	}
}

// measuredField creates a structured base value with a derived text sibling.
func measuredField(name fieldName, textLabel string, measure fieldMeasure, valueType fieldValueType) fieldSpec {
	spec := canonicalField(name, textLabel, measure, valueType)
	spec.Options.ShowText = false
	spec.StringSibling = name + "/String"
	return spec
}

// lookupFieldSpec returns the registered policy for name in kind, including text siblings.
func lookupFieldSpec(kind StreamKind, name fieldName) (fieldSpec, bool) {
	if base, ok := strings.CutSuffix(string(name), "/String"); ok {
		baseName := fieldName(base)
		base, ok := canonicalFieldDefinitions[baseName]
		if !ok {
			return fieldSpec{}, false
		}
		return fieldSpec{
			Name:      name,
			Measure:   fieldMeasureNone,
			Options:   fieldOptions{ShowText: true, ValueType: fieldValueString},
			Order:     textFieldOrder(kind, base.TextLabel),
			TextLabel: base.TextLabel,
		}, true
	}
	spec, ok := canonicalFieldDefinitions[name]
	if !ok {
		return fieldSpec{}, false
	}
	spec.Order = structuredFieldOrder(kind, spec.StructuredKey)
	return spec, true
}

// textFieldName maps a legacy display label to its canonical field or dynamic text field.
func textFieldName(kind StreamKind, label string) (fieldName, fieldSpec, bool) {
	if baseName, ok := textLabelCanonicalNames[label]; ok {
		base := canonicalFieldDefinitions[baseName]
		name := baseName
		if base.StringSibling != "" {
			name = base.StringSibling
		}
		spec, _ := lookupFieldSpec(kind, name)
		return name, spec, true
	}
	name := fieldName("Text/" + label)
	return name, fieldSpec{
		Name:      name,
		Options:   fieldOptions{ShowText: true, ValueType: fieldValueString},
		Order:     textFieldOrder(kind, label),
		TextLabel: label,
	}, false
}

// structuredFieldSpec returns the canonical or ordered dynamic definition for key.
func structuredFieldSpec(kind StreamKind, key string) (fieldSpec, bool) {
	if spec, ok := lookupFieldSpec(kind, fieldName(key)); ok {
		return spec, true
	}
	_, knownOrder := structuredFieldOrderPolicy(kind)[key]
	return fieldSpec{
		Name:          fieldName(key),
		Options:       fieldOptions{ShowStructured: true, ShowXML: true, ValueType: fieldValueString},
		StructuredKey: key,
		Order:         structuredFieldOrder(kind, key),
	}, knownOrder
}

// textFieldOrder returns the established display order, after all known fields for unknown labels.
func textFieldOrder(kind StreamKind, label string) int {
	order := textFieldOrderPolicy(kind)
	if value, ok := order[label]; ok {
		return value
	}
	return 1 << 20
}

// structuredFieldOrder returns the established structured order, after known
// keys when unknown.
func structuredFieldOrder(kind StreamKind, key string) int {
	if value, ok := structuredFieldOrderPolicy(kind)[key]; ok {
		return value
	}
	return 1 << 20
}
