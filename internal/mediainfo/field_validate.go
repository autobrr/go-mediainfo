package mediainfo

import "fmt"

// validateFieldStore verifies that both structured projections have unique, valid fields.
func validateFieldStore(store *fieldStore) error {
	if store == nil {
		return nil
	}
	ensureLegacyXMLProjection(store)
	store.projectionMu.RLock()
	defer store.projectionMu.RUnlock()
	for streamIndex, stream := range store.streams {
		for _, target := range []structuredProjectionTarget{structuredProjectionJSON, structuredProjectionXML} {
			seen := make(map[string]struct{})
			for _, entry := range stream.Fields {
				visible := target == structuredProjectionJSON && entry.Options.ShowStructured || target == structuredProjectionXML && entry.Options.ShowXML
				if !visible {
					continue
				}
				key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
				if _, exists := seen[key]; exists {
					return fmt.Errorf("stream %d %s has duplicate structured field %q", streamIndex, stream.Kind, key)
				}
				seen[key] = struct{}{}
				if entry.Node != nil && entry.Node.Kind == structuredRaw {
					return fmt.Errorf("stream %d %s field %q contains invalid structured JSON", streamIndex, stream.Kind, key)
				}
			}
		}
	}
	return nil
}
