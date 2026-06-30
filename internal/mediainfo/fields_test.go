package mediainfo

import "testing"

func TestAppendFieldUniquePreservesExistingField(t *testing.T) {
	fields := []Field{
		{Name: "Duration", Value: "1 s"},
		{Name: "Stream size", Value: "10 MiB (50%)"},
	}

	fields = appendFieldUnique(fields, Field{Name: "Stream size", Value: "9 MiB (45%)"})

	count := 0
	for _, field := range fields {
		if field.Name == "Stream size" {
			count++
			if field.Value != "10 MiB (50%)" {
				t.Fatalf("Stream size value = %q, want existing value", field.Value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("Stream size field count = %d, want 1", count)
	}
}
