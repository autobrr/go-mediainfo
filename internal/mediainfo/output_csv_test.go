package mediainfo

import "testing"

func TestRenderCSVSingleReport(t *testing.T) {
	report := Report{
		General: Stream{
			Kind:   StreamGeneral,
			Fields: []Field{{Name: "Format", Value: "MPEG-4"}},
		},
		Streams: []Stream{
			{Kind: StreamAudio, Fields: []Field{{Name: "Format", Value: "AAC"}}},
			{Kind: StreamAudio, Fields: []Field{{Name: "Format", Value: "AC-3"}}},
		},
	}

	output := RenderCSV([]Report{report})
	expected := "General\nFormat,MPEG-4\n\nAudio #1\nFormat,AAC\n\nAudio #2\nFormat,AC-3\n\n"
	if output != expected {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRenderCSVMultipleReports(t *testing.T) {
	reportA := Report{
		General: Stream{
			Kind:   StreamGeneral,
			Fields: []Field{{Name: "Format", Value: "MPEG-4"}},
		},
	}
	reportB := Report{
		General: Stream{
			Kind:   StreamGeneral,
			Fields: []Field{{Name: "Format", Value: "MP3"}},
		},
	}

	output := RenderCSV([]Report{reportA, reportB})
	expected := "General\nFormat,MPEG-4\n\nGeneral\nFormat,MP3\n\n"
	if output != expected {
		t.Fatalf("unexpected output:\n%s", output)
	}
}
