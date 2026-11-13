package mediainfo

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mp4MetaForTest(handler string, keys []string, items []byte, full bool) []byte {
	var b, h, k bytes.Buffer
	if full {
		b.Write(make([]byte, 4))
	}
	h.Write(make([]byte, 8))
	h.WriteString(handler)
	h.Write(make([]byte, 12))
	writeMP4Box(&b, "hdlr", h.Bytes())
	k.Write(make([]byte, 4))
	_ = binary.Write(&k, binary.BigEndian, uint32(len(keys)))
	for _, key := range keys {
		_ = binary.Write(&k, binary.BigEndian, uint32(len(key)+8))
		k.WriteString("mdta")
		k.WriteString(key)
	}
	if handler == "mdta" {
		writeMP4Box(&b, "keys", k.Bytes())
	}
	writeMP4Box(&b, "ilst", items)
	return b.Bytes()
}
func mp4MetaItemForTest(key string, kind uint32, value []byte) []byte {
	var d, b, out bytes.Buffer
	_ = binary.Write(&d, binary.BigEndian, kind)
	d.Write(make([]byte, 4))
	d.Write(value)
	writeMP4Box(&b, "data", d.Bytes())
	writeMP4Box(&out, key, b.Bytes())
	return out.Bytes()
}
func mp4MetaIndexForTest(i uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], i)
	return string(b[:])
}
func TestMP4MetadataTitleAndCollection(t *testing.T) {
	items := append(mp4MetaItemForTest("\xa9nam", 1, []byte("  Title  \x00")), mp4MetaItemForTest("tvsh", 1, []byte("Series"))...)
	items = append(items, mp4MetaItemForTest("\xa9alb", 1, []byte(" Album "))...)
	var u bytes.Buffer
	writeMP4Box(&u, "meta", mp4MetaForTest("mdir", nil, items, true))
	got := parseMP4UserMetadata(u.Bytes())
	want := []Field{{Name: "Title", Value: "Title"}, {Name: "Album", Value: " Album "}, {Name: "Collection", Value: "Series"}}
	if len(got) != len(want) {
		t.Fatalf("fields=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fields=%v", got)
		}
	}
}
func TestMP4IndexedMetadata(t *testing.T) {
	var fps [4]byte
	binary.BigEndian.PutUint32(fps[:], math.Float32bits(60))
	items := append(mp4MetaItemForTest(mp4MetaIndexForTest(2), 23, fps[:]), mp4MetaItemForTest(mp4MetaIndexForTest(1), 1, []byte("14"))...)
	items = append(items, mp4MetaItemForTest(mp4MetaIndexForTest(2), 23, []byte{0, 0})...)
	for _, full := range []bool{false, true} {
		b := embeddedAssetBudget{}
		m := parseMP4Meta(mp4MetaForTest("mdta", []string{"com.android.version", "com.android.capture.fps"}, items, full), &b)
		if m.frameRateReal != 60 {
			t.Fatalf("full=%v fps=%v", full, m.frameRateReal)
		}
		if mp4MetadataTestValue(m.fields, "Encoded_OperatingSystem_Version") != "14" {
			t.Fatalf("fields=%v", m.fields)
		}
	}
}
func TestMP4MetadataTypedValues(t *testing.T) {
	tests := []struct {
		kind uint32
		data []byte
		want string
		ok   bool
	}{
		{1, []byte("text\x00"), "text", true}, {21, []byte{0xff}, "-1", true}, {22, []byte{0xff}, "255", true},
		{21, []byte{1, 2, 3}, "", false}, {23, []byte{0x7f, 0x80, 0, 0}, "", false}, {24, make([]byte, 7), "", false},
	}
	for _, tt := range tests {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, tt.kind)
		b.Write(make([]byte, 4))
		b.Write(tt.data)
		got, ok := decodeMP4MetadataValue(b.Bytes())
		if got != tt.want || ok != tt.ok {
			t.Fatalf("kind %d: %q,%v", tt.kind, got, ok)
		}
	}
}
func TestMP4MetadataLocation(t *testing.T) {
	for _, tt := range []struct{ in, want string }{{"+12.500-003.250+004.0/", "12.500°N 3.250°W 4.0m"}, {"-00.00+000.00/", ""}, {"unparsed", "unparsed"}, {"+12/", "+12/"}, {"/bad", "/bad"}, {"12°3'4\"N 34°5'6\"E", "12°03'04\"N 34°05'06\"E"}, {"+12.0+034.0-002.0/", "12.0°N 34.0°E -002.0m"}} {
		if got := mp4RecordedLocation(tt.in); got != tt.want {
			t.Fatalf("%q -> %q, want %q", tt.in, got, tt.want)
		}
	}
}

func mp4MetadataTestValue(fields []Field, key string) string {
	for _, f := range fields {
		if f.Name == key {
			return f.Value
		}
	}
	return ""
}

func TestMP4MetadataScopeAndHandlerReadOrder(t *testing.T) {
	handler := append(make([]byte, 8), []byte("vide")...)
	handler = append(handler, make([]byte, 12)...)
	handler = append(handler, []byte("CustomControl\x00")...)
	var mdia, track bytes.Buffer
	writeMP4Box(&mdia, "hdlr", handler)
	writeMP4Box(&track, "mdia", mdia.Bytes())
	tm := mp4MetaForTest("mdta", []string{"com.apple.quicktime.displayname"}, mp4MetaItemForTest(mp4MetaIndexForTest(1), 1, []byte("TrackControl")), false)
	writeMP4Box(&track, "meta", tm)
	gm := mp4MetaForTest("mdta", []string{"com.apple.quicktime.displayname"}, mp4MetaItemForTest(mp4MetaIndexForTest(1), 1, []byte("GeneralControl")), false)
	for _, before := range []bool{true, false} {
		var movie bytes.Buffer
		if before {
			writeMP4Box(&movie, "meta", gm)
		}
		writeMP4Box(&movie, "trak", track.Bytes())
		if !before {
			writeMP4Box(&movie, "meta", gm)
		}
		info, ok := parseMoov(movie.Bytes())
		if !ok || len(info.Tracks) != 1 {
			t.Fatal("track absent")
		}
		tr := info.Tracks[0]
		if tr.handlerTitleEligible == before || tr.HandlerName != "CustomControl" {
			t.Fatalf("before=%v eligible=%v handler=%q", before, tr.handlerTitleEligible, tr.HandlerName)
		}
		if mp4MetadataTestValue(info.General, "Title") != "GeneralControl" || mp4MetadataTestValue(tr.metadata.fields, "Title") != "TrackControl" {
			t.Fatal("metadata leaked between scopes")
		}
	}
	for _, name := range []string{"VideoHandler", "Audio handler", "Gestionnaire test", "Module test", "Gestor test", "Procedura test"} {
		if mp4HandlerTitleEligible(name) {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, name := range []string{"Custom", "handler", "videoHANDLER", "GestorX"} {
		if !mp4HandlerTitleEligible(name) {
			t.Fatalf("rejected %q", name)
		}
	}
}

func TestMP4MetadataIndexBoundsAndMappings(t *testing.T) {
	keys := []string{"PUBLISHER", "date_recorded", "iTunNORM", "unknown", "com.apple.quicktime.comment"}
	var items []byte
	for _, v := range []struct {
		i     uint32
		value string
	}{{4, "first"}, {1, "publisher"}, {4, "last"}, {2, "2024:01:02"}, {3, "ignored"}, {0, "bad"}, {8, "bad"}, {5, "comment"}} {
		items = append(items, mp4MetaItemForTest(mp4MetaIndexForTest(v.i), 1, []byte(v.value))...)
	}
	data := mp4MetaForTest("mdta", keys, items, true)
	b := embeddedAssetBudget{}
	m := parseMP4Meta(data, &b)
	if mp4MetadataTestValue(m.fields, "Publisher") != "publisher" || mp4MetadataTestValue(m.fields, "Recorded_Date") != "2024-01-02" || mp4MetadataTestValue(m.fields, "Comment") != "comment" {
		t.Fatalf("fields=%v", m.fields)
	}
	if len(m.extras) != 1 || m.extras[0].Key != "unknown" || m.extras[0].Val != "last" {
		t.Fatalf("extras=%v", m.extras)
	}
	for _, bad := range [][]byte{data[:len(data)-1], bytes.Replace(data, []byte("mdta"), []byte("mdir"), 1), bytes.Replace(data, []byte("\x00\x00\x00\x05"), []byte("\xff\xff\xff\xff"), 1)} {
		budget := embeddedAssetBudget{}
		m := parseMP4Meta(bad, &budget)
		if len(m.fields) != 0 || len(m.extras) != 0 {
			t.Fatal("accepted malformed or wrong-handler metadata")
		}
	}
	// A subsequent meta atom must not reuse the previous keys table.
	var noKeys bytes.Buffer
	hdlr := append(make([]byte, 8), []byte("mdta")...)
	writeMP4Box(&noKeys, "hdlr", hdlr)
	writeMP4Box(&noKeys, "ilst", items)
	m = parseMP4Meta(noKeys.Bytes(), &b)
	if len(m.fields) != 0 || len(m.extras) != 0 {
		t.Fatal("leaked key table")
	}
}

func TestMP4BMPCoverBounds(t *testing.T) {
	bmp := make([]byte, 58)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[2:6], 58)
	binary.LittleEndian.PutUint32(bmp[10:14], 54)
	binary.LittleEndian.PutUint32(bmp[14:18], 40)
	binary.LittleEndian.PutUint32(bmp[18:22], 1)
	binary.LittleEndian.PutUint32(bmp[22:26], 1)
	binary.LittleEndian.PutUint16(bmp[26:28], 1)
	binary.LittleEndian.PutUint16(bmp[28:30], 24)
	for _, kind := range []uint32{13, 14, 27, 99} {
		var udta bytes.Buffer
		writeMP4Box(&udta, "meta", mp4MetaForTest("mdir", nil, mp4MetaItemForTest("covr", kind, bmp), true))
		got := parseMP4UnknownUserMetadata(udta.Bytes())
		if (len(got) == 0) != (kind != 99) {
			t.Fatalf("kind=%d extras=%v", kind, got)
		}
	}
	for n := 0; n < 54; n++ {
		if validMP4BMPCover(bmp[:n]) {
			t.Fatalf("accepted %d bytes", n)
		}
	}
	bad := append([]byte(nil), bmp...)
	binary.LittleEndian.PutUint32(bad[14:18], math.MaxUint32)
	if validMP4BMPCover(bad) {
		t.Fatal("accepted oversized DIB")
	}
}

func TestMP4TraditionalLocationIsNotIndexedMetadata(t *testing.T) {
	value := "+12.500+034.000/"
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload, uint16(len(value)))
	payload = append(payload, []byte(value)...)
	var udta bytes.Buffer
	writeMP4Box(&udta, "\xa9xyz", payload)
	m := mp4Metadata{}
	b := embeddedAssetBudget{}
	parseMP4MetadataChildren(udta.Bytes(), &m, &b)
	if got := mp4MetadataTestValue(m.fields, "Recorded_Location"); got != "12.500°N 34.000°E" {
		t.Fatalf("location=%q", got)
	}
}

func TestMP4MetadataAnalysisOutput(t *testing.T) {
	var movie, file bytes.Buffer
	keys := []string{"com.android.version", "com.android.capture.fps", "com.apple.quicktime.version", "com.apple.quicktime.displayname", "iTunMOVI", "com.apple.quicktime.location.ISO6709", "com.universaladid.idregistry", "com.universaladid.idvalue", "com.example.metadata"}
	values := []string{"14", "60", "7.7.9 extra", " General ", "<plist/>", "+12.5+034.0/", "ad-id.org", "Ad123", "Synthetic extra"}
	var items []byte
	for i, value := range values {
		items = append(items, mp4MetaItemForTest(mp4MetaIndexForTest(uint32(i+1)), 1, []byte(value))...)
	}
	writeMP4Box(&movie, "meta", mp4MetaForTest("mdta", keys, items, true))
	for _, kind := range []string{"vide", "soun"} {
		h := make([]byte, 24)
		copy(h[8:], kind)
		copy(h[12:], "appl")
		h = append(h, []byte("Custom handler title\x00")...)
		var media, track bytes.Buffer
		writeMP4Box(&media, "hdlr", h)
		writeMP4Box(&track, "mdia", media.Bytes())
		trackExtra := mp4MetaItemForTest(mp4MetaIndexForTest(1), 1, []byte("Track extra"))
		writeMP4Box(&track, "meta", mp4MetaForTest("mdta", []string{"com.example.track"}, trackExtra, true))
		writeMP4Box(&movie, "trak", track.Bytes())
	}
	writeMP4Box(&file, "ftyp", []byte("isom"))
	writeMP4Box(&file, "moov", movie.Bytes())
	path := filepath.Join(t.TempDir(), "metadata.mp4")
	if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"Title": "General", "Encoded_OperatingSystem_Name": "Android", "Encoded_OperatingSystem_Version": "14", "Encoded_Library": "Apple QuickTime 7.7.9", "Recorded_Location": "12.5°N 34.0°E"} {
		if got, _ := canonicalSeedValue(report.General, fieldName(key)); got != want {
			t.Errorf("%s=%q, want %q", key, got, want)
		}
	}
	reports := []Report{report}
	for _, output := range []string{RenderText(reports), RenderCSV(reports), RenderHTML(reports)} {
		if strings.Contains(output, "Universal Ad ID registry") || strings.Contains(output, "Universal Ad ID value") || !strings.Contains(output, "Ad123 (ad-id.org)") {
			t.Errorf("incorrect UniversalAdID friendly visibility: %s", output)
		}
		if !strings.Contains(output, "Writing operating system") || !strings.Contains(output, "Google Android 14") || strings.Contains(output, "Encoded_OperatingSystem_") {
			t.Errorf("incorrect operating system presentation: %s", output)
		}
	}
	if raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}); !strings.Contains(raw, padRawTextLabel("FrameRate_Real/String", 33)+": 60.000 fps") {
		t.Errorf("incorrect raw capture frame rate: %s", raw)
	}
	if raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}); !strings.Contains(raw, "Encoded_OperatingSystem/String") || strings.Contains(raw, "Encoded_OperatingSystem_CompanyName") {
		t.Errorf("incorrect raw operating system presentation: %s", raw)
	}
	if json := RenderJSON(reports); !strings.Contains(json, `"UniversalAdID_Registry":"ad-id.org"`) || !strings.Contains(json, `"UniversalAdID_Value":"Ad123"`) || strings.Contains(json, `"UniversalAdID/String":`) || strings.Contains(json, `"UniversalAdID_String":`) {
		t.Errorf("missing structured UniversalAdID components: %s", json)
	}
	if json := RenderJSON(reports); !strings.Contains(json, `"com_example_metadata":"Synthetic extra"`) || strings.Contains(json, `"com.example.metadata":`) || !strings.Contains(json, `"com_example_track":"Track extra"`) {
		t.Errorf("incorrect metadata JSON name: %s", json)
	}
	if raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}); !strings.Contains(raw, "com.example.metadata") || strings.Contains(raw, "com_example_metadata") {
		t.Errorf("metadata normalization changed raw name: %s", raw)
	}
	for _, output := range []string{RenderText(reports), RenderHTML(reports)} {
		if !strings.Contains(output, "com.example.metadata") || !strings.Contains(output, "Synthetic extra") || strings.Count(output, "Synthetic extra") != 1 {
			t.Errorf("unknown General metadata lacks one friendly row: %s", output)
		}
		if strings.Count(output, "Track extra") != 2 {
			t.Errorf("unknown track metadata lacks one friendly row per track: %s", output)
		}
	}
	if raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"}); strings.Count(raw, "Synthetic extra") != 1 || strings.Count(raw, "Track extra") != 2 {
		t.Errorf("unknown metadata raw rows are not unique: %s", raw)
	}
	if len(report.Streams) != 2 {
		t.Fatalf("streams=%d", len(report.Streams))
	}
	for _, stream := range report.Streams {
		if stream.Kind == StreamVideo {
			if value, _ := canonicalSeedTextValue(stream, "Real frame rate"); value == "" {
				t.Error("capture rate lacks its own text label")
			}
			if value, _ := canonicalSeedTextValue(stream, "Original frame rate"); value != "" {
				t.Error("capture rate aliases original frame rate")
			}
		}
		fps, _ := canonicalSeedValue(stream, "FrameRate_Real")
		if (stream.Kind == StreamVideo && fps != "60.000") || (stream.Kind == StreamAudio && fps != "") {
			t.Errorf("%s capture fps=%q", stream.Kind, fps)
		}
		if title, _ := canonicalSeedValue(stream, "Title"); title != "" {
			t.Errorf("late handler title=%q", title)
		}
	}
}

func TestMP4MetadataExtraNamesPreserveCodecCollisions(t *testing.T) {
	for _, kind := range []StreamKind{StreamGeneral, StreamAudio} {
		builder := newCanonicalStreamBuilder(kind)
		builder.StructuredNode("extra", structuredObjectFromKVs([]jsonKV{{Key: "a_b", Val: "codec"}, {Key: "a.b", Val: "metadata"}}))
		metadata := mp4Metadata{extras: []jsonKV{{Key: "a.b", Val: "metadata"}}}
		metadata.applyExtraNames(builder, 1)
		stream := builder.Snapshot(canonicalStreamPolicy{})
		report := Report{General: Stream{Kind: StreamGeneral}}
		if kind == StreamGeneral {
			report.General = stream
		} else {
			report.Streams = []Stream{stream}
		}
		attachCanonicalStore(&report)
		reports := []Report{report}
		json := RenderJSON(reports)
		if !strings.Contains(json, `"a_b":"codec"`) || !strings.Contains(json, `"a_b_612E62":"metadata"`) {
			t.Errorf("%s collision lost a value: %s", kind, json)
		}
		raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
		if !strings.Contains(raw, "a.b") || strings.Contains(raw, "a_b_612E62") {
			t.Errorf("%s raw names changed: %s", kind, raw)
		}
		if RenderJSON(reports) != json {
			t.Fatal("render changed metadata projection")
		}
	}
}

func TestMP4MetadataExactCodecCollision(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.StructuredNode("extra", structuredObjectFromKVs([]jsonKV{{Key: "MD5_Unencoded", Val: "metadata"}, {Key: "MD5_Unencoded_4D44355F556E656E636F646564", Val: "other metadata"}, {Key: "MD5_Unencoded", Val: "codec"}}))
	metadata := mp4Metadata{extras: []jsonKV{{Key: "MD5_Unencoded", Val: "metadata"}, {Key: "MD5_Unencoded_4D44355F556E656E636F646564", Val: "other metadata"}}}
	metadata.applyExtraNames(builder, 0)
	stream := builder.Snapshot(canonicalStreamPolicy{})
	report := Report{General: Stream{Kind: StreamGeneral}, Streams: []Stream{stream}}
	attachCanonicalStore(&report)
	reports := []Report{report}
	json := RenderJSON(reports)
	if !strings.Contains(json, `"MD5_Unencoded":"codec"`) || !strings.Contains(json, `"MD5_Unencoded_4D44355F556E656E636F646564":"metadata"`) {
		t.Fatalf("collision lost codec identity: %s", json)
	}
	raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
	if !strings.Contains(raw, "codec") || !strings.Contains(raw, "other metadata") || !strings.Contains(raw, "MD5_Unencoded_4D44355F556E656E636F646564_Tag") {
		t.Fatalf("raw lost collision value: %s", raw)
	}
}

func TestMP4MetadataMalformedLocationRemainsRaw(t *testing.T) {
	for _, value := range []string{"+999+999/", "+90.1+000/", "+12.5+180.1/", "+12.5+034.0/junk", "+12.5+034.0//", "+12.5+034.0+abc/", "+.5+034/", "+12.+034/"} {
		if got := mp4RecordedLocation(value); got != value {
			t.Errorf("%q became %q", value, got)
		}
	}
	if got := mp4RecordedLocation("-90+180/"); got != "90°S 180°E" {
		t.Errorf("valid boundary became %q", got)
	}
}

func TestMP4VendorIncludesAllHandlers(t *testing.T) {
	for _, tt := range []struct{ name, mediaVendor, dataVendor, extraVendor, want string }{
		{"data handler only", "", "appl", "", "Apple QuickTime"},
		{"matching handlers", "appl", "appl", "", "Apple QuickTime"},
		{"conflicting data handler", "niko", "appl", "", ""},
		{"unsupported track conflict", "appl", "", "niko", ""},
		{"unsupported track vendor", "", "", "niko", "Nikon"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(kind, vendor string) []byte {
				h := make([]byte, 24)
				copy(h[8:], kind)
				copy(h[12:], vendor)
				return h
			}
			var movie, media, minf, track, file bytes.Buffer
			writeMP4Box(&media, "hdlr", handler("vide", tt.mediaVendor))
			writeMP4Box(&minf, "hdlr", handler("url ", tt.dataVendor))
			writeMP4Box(&media, "minf", minf.Bytes())
			writeMP4Box(&track, "mdia", media.Bytes())
			writeMP4Box(&movie, "trak", track.Bytes())
			if tt.extraVendor != "" {
				media.Reset()
				track.Reset()
				writeMP4Box(&media, "hdlr", handler("xxxx", tt.extraVendor))
				writeMP4Box(&track, "mdia", media.Bytes())
				writeMP4Box(&movie, "trak", track.Bytes())
			}
			writeMP4Box(&file, "ftyp", []byte("qt  "))
			writeMP4Box(&file, "moov", movie.Bytes())
			path := filepath.Join(t.TempDir(), "vendor.mov")
			if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := canonicalSeedValue(report.General, "Encoded_Library"); got != tt.want {
				t.Errorf("vendor=%q want %q", got, tt.want)
			}
			if len(report.Streams) != 1 {
				t.Errorf("unsupported track escaped: %d streams", len(report.Streams))
			}
		})
	}
}

func TestMP4BMPCoverRenderedVisibility(t *testing.T) {
	bmp := make([]byte, 58)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[2:6], 58)
	binary.LittleEndian.PutUint32(bmp[10:14], 54)
	binary.LittleEndian.PutUint32(bmp[14:18], 40)
	binary.LittleEndian.PutUint32(bmp[18:22], 1)
	binary.LittleEndian.PutUint32(bmp[22:26], 1)
	binary.LittleEndian.PutUint16(bmp[26:28], 1)
	binary.LittleEndian.PutUint16(bmp[28:30], 24)
	path := filepath.Join(t.TempDir(), "cover.mp4")
	var referenceJSON, referenceRaw string
	for _, kind := range []uint32{13, 14, 27, 99} {
		items := append(mp4MetaItemForTest("\xa9nam", 1, []byte("Cover title")), mp4MetaItemForTest("covr", kind, bmp)...)
		items = append(items, mp4MetaItemForTest("\xa9alb", 1, []byte("Cover album"))...)
		var udta, movie, file bytes.Buffer
		writeMP4Box(&udta, "meta", mp4MetaForTest("mdir", nil, items, true))
		writeMP4Box(&movie, "udta", udta.Bytes())
		mvhd := make([]byte, 20)
		binary.BigEndian.PutUint32(mvhd[12:16], 1000)
		binary.BigEndian.PutUint32(mvhd[16:20], 1000)
		writeMP4Box(&movie, "mvhd", mvhd)
		writeMP4Box(&file, "ftyp", []byte("isom"))
		writeMP4Box(&file, "moov", movie.Bytes())
		if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		// Compare cover behavior, not timestamps from rewriting the same fixture.
		stamp := time.Unix(1700000000, 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		report, err := AnalyzeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		reports := []Report{report}
		json := RenderJSON(reports)
		raw := RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})
		if len(report.Streams) != 0 {
			t.Fatal("cover became media stream")
		}
		if kind == 13 {
			referenceJSON, referenceRaw = json, raw
		}
		if kind != 99 && (json != referenceJSON || raw != referenceRaw) {
			t.Errorf("kind %d changed JPEG/PNG visibility or order", kind)
		}
		for _, output := range []string{json, raw} {
			if !strings.Contains(output, "Cover album") || !strings.Contains(output, "Cover title") {
				t.Fatal("lost surrounding metadata")
			}
			if kind == 99 {
				if strings.Index(output, "covr") < strings.Index(output, "Cover album") || !strings.Contains(output, "Unknown kind of value!") {
					t.Errorf("unknown fallback/order changed: %s", output)
				}
			} else if strings.Contains(output, "covr") || strings.Contains(output, "Unknown kind of value!") {
				t.Errorf("kind%d leaked cover: %s", kind, output)
			}
		}
	}
}

func TestMP4VideoMetadataPreservesCodecExtras(t *testing.T) {
	for _, codec := range []struct {
		sample, box, want string
		payload           []byte
	}{
		{"avc1", "avcC", "avcC", []byte{1, 66, 0, 30, 255, 224, 0}},
		{"hvc1", "hvcC", "hvcC+dvvC", make([]byte, 23)},
	} {
		for _, headerExtra := range []bool{false, true} {
			for _, metadata := range []bool{false, true} {
				var config, track, movie, file bytes.Buffer
				writeMP4Box(&config, codec.box, codec.payload)
				if codec.sample == "hvc1" {
					writeMP4Box(&config, "dvvC", []byte{1, 0, 16, 13, 16})
				}
				track.Write(buildTrackWithStsd("vide", codec.sample, config.Bytes()))
				if metadata {
					writeMP4Box(&track, "meta", mp4MetaForTest("mdta", []string{"com.example.track"}, mp4MetaItemForTest(mp4MetaIndexForTest(1), 1, []byte("Preserved tag")), true))
				}
				writeMP4Box(&movie, "trak", track.Bytes())
				if headerExtra {
					var udta bytes.Buffer
					writeMP4Box(&udta, "meta", mp4MetaForTest("mdir", nil, mp4MetaItemForTest("\xa9too", 1, []byte("GPAC-test")), true))
					writeMP4Box(&movie, "udta", udta.Bytes())
				}
				writeMP4Box(&file, "ftyp", []byte("isom"))
				writeMP4Box(&file, "moov", movie.Bytes())
				path := filepath.Join(t.TempDir(), "codec-extra.mp4")
				if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				report, err := AnalyzeFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(report.Streams) != 1 {
					t.Fatal("missing video")
				}
				node := canonicalSeedStructuredNode(&report.Streams[0], "extra")
				if node == nil {
					t.Fatal("missing video extra")
				}
				wantKeys := []string{}
				if headerExtra {
					wantKeys = append(wantKeys, "mdhd_Duration")
				}
				wantKeys = append(wantKeys, "CodecConfigurationBox")
				if metadata {
					wantKeys = append(wantKeys, "com_example_track")
				}
				if len(node.Object) != len(wantKeys) {
					t.Errorf("%s extra count=%d, want %d", codec.sample, len(node.Object), len(wantKeys))
				} else {
					for i, key := range wantKeys {
						if node.Object[i].Key != key {
							t.Errorf("%s extra[%d]=%s, want %s", codec.sample, i, node.Object[i].Key, key)
						}
					}
				}
				codecCount, tagCount := 0, 0
				for _, member := range node.Object {
					if member.Key == "CodecConfigurationBox" {
						codecCount++
						if member.Value.Text != codec.want {
							t.Errorf("codec value=%q", member.Value.Text)
						}
					}
					if member.Key == "com_example_track" {
						tagCount++
					}
				}
				if codecCount != 1 || (metadata && tagCount != 1) || (!metadata && tagCount != 0) {
					t.Errorf("headerExtra=%v metadata=%v codec/tag counts=%d/%d", headerExtra, metadata, codecCount, tagCount)
				}
				reports := []Report{report}
				for _, check := range []struct{ output, want string }{{RenderJSON(reports), `"CodecConfigurationBox":"` + codec.want + `"`}, {RenderXML(reports), "<CodecConfigurationBox>" + codec.want + "</CodecConfigurationBox>"}} {
					if strings.Count(check.output, check.want) != 1 {
						t.Errorf("headerExtra=%v metadata=%v missing/duplicate codec: %s", headerExtra, metadata, check.output)
					}
					if metadata && !strings.Contains(check.output, "Preserved tag") {
						t.Fatal("lost metadata tag")
					}
				}
			}
		}
	}
}

func mp4MetadataFileForTest(t *testing.T, keys, values []string, trackKeys, trackValues []string) Report {
	t.Helper()
	var movie, file bytes.Buffer
	var items []byte
	for i, value := range values {
		items = append(items, mp4MetaItemForTest(mp4MetaIndexForTest(uint32(i+1)), 1, []byte(value))...)
	}
	writeMP4Box(&movie, "meta", mp4MetaForTest("mdta", keys, items, true))
	h := make([]byte, 24)
	copy(h[8:], "vide")
	h = append(h, 0)
	var media, track bytes.Buffer
	writeMP4Box(&media, "hdlr", h)
	writeMP4Box(&track, "mdia", media.Bytes())
	var trackItems []byte
	for i, value := range trackValues {
		trackItems = append(trackItems, mp4MetaItemForTest(mp4MetaIndexForTest(uint32(i+1)), 1, []byte(value))...)
	}
	writeMP4Box(&track, "meta", mp4MetaForTest("mdta", trackKeys, trackItems, true))
	writeMP4Box(&movie, "trak", track.Bytes())
	writeMP4Box(&file, "ftyp", []byte("isom"))
	writeMP4Box(&file, "moov", movie.Bytes())
	path := filepath.Join(t.TempDir(), "metadata.mp4")
	if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestMP4MetadataSourceRowsKeepIdentity(t *testing.T) {
	report := mp4MetadataFileForTest(t,
		[]string{"File size", "Format"}, []string{"tag size", "tag format"},
		[]string{"Format", "com.apple.quicktime.copyright"}, []string{"track format", "track copyright"})
	reports := []Report{report}
	for name, output := range map[string]string{"text": RenderText(reports), "html": RenderHTML(reports), "raw": RenderTextWithOptions(reports, TextRenderOptions{Language: "raw"})} {
		for _, value := range []string{"tag size", "tag format", "track format", "track copyright"} {
			if got := strings.Count(output, value); got != 1 {
				t.Errorf("%s shows %q %d times, want 1:\n%s", name, value, got, output)
			}
		}
		if got := strings.Count(output, "MPEG-4"); got != 1 {
			t.Errorf("%s General format rows=%d:\n%s", name, got, output)
		}
		if got := strings.Count(output, ": Video") + strings.Count(output, "<td>Video</td>"); got != 1 {
			t.Errorf("%s Video format rows=%d:\n%s", name, got, output)
		}
	}
	json := RenderJSON(reports)
	for _, want := range []string{`"File_size":"tag size"`, `"extra":{"Copyright":"track copyright","Format":"track format"}`, `"FileSize":"`, `"Format":"Video"`} {
		if !strings.Contains(json, want) {
			t.Errorf("JSON lacks %s: %s", want, json)
		}
	}
	if strings.Count(json, `"Copyright"`) != 1 {
		t.Errorf("track metadata outside the Video schema is a top-level field: %s", json)
	}
}

func TestMP4GeneralDatesUseReferenceAdapter(t *testing.T) {
	for _, tt := range []struct{ value, want string }{
		{"2026-11-12T10:00:00Z", "2026-11-12 10:00:00 UTC"},
		{"Thu Nov 12 10:00:00 2026", "2026-11-12 10:00:00"},
		// v23.04 keeps +hhmm offsets; only the Matroska path adopts v26.10.
		{"2026-11-12T10:00:00+0100", "2026-11-12T10:00:00+0100"},
		{"unknown", ""},
	} {
		report := mp4MetadataFileForTest(t, []string{"DATE", "date_recorded"}, []string{tt.value, tt.value}, nil, nil)
		for _, key := range []fieldName{"Encoded_Date", "Recorded_Date"} {
			if got, _ := canonicalSeedValue(report.General, key); got != tt.want {
				t.Errorf("%s %q=%q, want %q", key, tt.value, got, tt.want)
			}
		}
	}
}
