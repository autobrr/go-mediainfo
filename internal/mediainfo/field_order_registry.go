package mediainfo

// unregisteredFieldOrder places genuine extras after standard table rows.
const unregisteredFieldOrder = 1 << 20

func makeStructuredFieldOrder(names ...string) map[string]int {
	order := make(map[string]int, len(names))
	for index, name := range names {
		order[name] = index
	}
	return order
}

func structuredFieldOrderPolicy(kind StreamKind) map[string]int {
	if table := reportKindPolicy(reportTables23, kind); table != nil {
		return table.Order
	}
	return nil
}

func structuredDVDFieldOrderPolicy(kind StreamKind) map[string]int {
	if table := reportKindPolicy(reportTables26, kind); table != nil {
		return table.Order
	}
	return nil
}

func structuredFieldOrderForContainer(kind StreamKind, containerFormat string) map[string]int {
	if containerFormat == "DVD Video" {
		return structuredDVDFieldOrderPolicy(kind)
	}
	return structuredFieldOrderPolicy(kind)
}

func isKnownStructuredField(kind StreamKind, name string) bool {
	_, ok := structuredFieldOrderPolicy(kind)[name]
	return ok && name != "extra" && name != "@type" && name != "@typeorder"
}
