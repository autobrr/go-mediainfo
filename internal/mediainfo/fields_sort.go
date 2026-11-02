package mediainfo

import "sort"

// sortFields keeps extras in producer order after the standard per-kind rows.
func sortFields(kind StreamKind, fields []Field) {
	sort.SliceStable(fields, func(i, j int) bool {
		return textFieldOrder(kind, fields[i].Name) < textFieldOrder(kind, fields[j].Name)
	})
}
