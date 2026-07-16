package mediainfo

import "sort"

// sortFields stably applies the registered display order, sorting unknown
// labels lexically after known fields.
func sortFields(kind StreamKind, fields []Field) {
	if kind == StreamMenu {
		return
	}
	order := textFieldOrderPolicy(kind)

	sort.SliceStable(fields, func(i, j int) bool {
		ai, aok := order[fields[i].Name]
		aj, bok := order[fields[j].Name]
		switch {
		case aok && bok:
			return ai < aj
		case aok:
			return true
		case bok:
			return false
		default:
			return fields[i].Name < fields[j].Name
		}
	})
}
