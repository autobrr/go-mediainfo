package mediainfo

import "testing"

func TestExposeWritingApplicationComponents(t *testing.T) {
	tests := []struct {
		name    string
		app     string
		version string
		want    bool
	}{
		{name: "mkvmerge 81", app: "mkvmerge", version: "81.0 ('Milliontown') 64-bit", want: true},
		{name: "mkvmerge 19", version: "19.0.0 ('Brave Captain') 64-bit", want: false},
		{name: "mkvmerge 94", version: "94.0 ('Initiate') 64-bit", want: true},
		{name: "mkvmerge 96", version: "96.0 ('It's My Life') 64-bit", want: true},
		{name: "mkvmerge 98", version: "98.0 ('Chonks') 64-bit", want: true},
		{name: "mkvmerge 99", version: "99.0 ('Buka') 64-bit", want: true},
		{name: "mkvmerge 97", version: "97.0 ('You Don't Have A Clue') 64-bit", want: false},
		{name: "invalid version", version: "development", want: false},
		{name: "non-mkvmerge", app: "ffmpeg", version: "81.0 ('Milliontown') 64-bit", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := test.app
			if app == "" {
				app = "mkvmerge"
			}
			if got := exposeWritingApplicationComponents(app, test.version); got != test.want {
				t.Fatalf("exposeWritingApplicationComponents() = %v, want %v", got, test.want)
			}
		})
	}
}
