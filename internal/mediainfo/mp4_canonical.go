package mediainfo

// mp4StructuredFacts provides typed staging for MP4 facts that require an
// exact public compatibility value.
type mp4StructuredFacts struct {
	canonicalStructuredFacts
}

// newMP4StructuredFacts imports every scalar from a sample-entry canonical
// seed and retains exact public compatibility values where they differ.
func newMP4StructuredFacts(seed []fieldEntry) *mp4StructuredFacts {
	facts := &mp4StructuredFacts{}
	for _, entry := range seed {
		if !entry.Options.ShowStructured || entry.Node != nil || entry.Value.Text == "" {
			continue
		}
		name := fieldName(firstNonEmpty(entry.StructuredKey, string(entry.Name)))
		facts.SetCanonical(name, entry.Value.Text)
	}
	return facts
}

// Set replaces one exact MP4 compatibility scalar.
func (f *mp4StructuredFacts) Set(name fieldName, value string) {
	if f == nil {
		return
	}
	f.SetSame(name, value)
}

// Get returns the last staged compatibility value for name.
func (f *mp4StructuredFacts) Get(name fieldName) string {
	if f == nil {
		return ""
	}
	return f.Canonical(name)
}
