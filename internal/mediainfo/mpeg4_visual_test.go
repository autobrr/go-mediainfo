package mediainfo

import (
	"strconv"
	"strings"
	"testing"
)

func TestParseMPEG4VOLBitRateUsesMediaInfoCombination(t *testing.T) {
	data := []byte{
		0x00, 0xc8, 0x0d, 0xc0, 0x01, 0xf0, 0x61, 0xc0,
		0x18, 0x40, 0x02, 0xa0, 0x00, 0x97, 0x53, 0x0a,
		0x20, 0x08, 0x60, 0x28, 0x30, 0x7f,
	}
	if got := parseMPEG4VOL(data).BitRateNominal; got != 9918000 {
		t.Fatalf("nominal bit rate = %d", got)
	}
}

func TestParseMPEG4VOLDefaultMPEGMatrix(t *testing.T) {
	data := []byte{
		0x08, 0xc8, 0x0d, 0x8b, 0xa9, 0x85,
		0x28, 0x04, 0x5a, 0x14, 0x46, 0x0f,
	}
	if got := parseMPEG4VOL(data).Matrix; got != "Default (MPEG)" {
		t.Fatalf("matrix = %q", got)
	}
}

func TestParseMPEG4VOLBufferSizeUsesMediaInfoUnits(t *testing.T) {
	tests := []struct {
		name string
		high uint32
		low  uint32
		want int64
	}{
		{name: "zero"},
		{name: "one low unit", low: 1, want: 2_048},
		{name: "low only", low: 7, want: 14_336},
		{name: "high only", high: 1, want: 67_108_864},
		// MediaInfoLib v26.05 and immutable DivX samples S243/S247/S249/S250/S258.
		{name: "official DivX buffer", high: 24, want: 1_610_612_736},
		{name: "both halves", high: 0x1234, low: 5, want: 312_727_316_480},
		{name: "maximum", high: 0x7FFF, low: 7, want: 2_198_956_161_024},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const bitRateHigh = uint32(0x123)
			const bitRateLow = uint32(0x456)
			got := parseMPEG4VOL(buildMPEG4VOLWithVBV(bitRateHigh, bitRateLow, test.high, test.low))
			if got.BufferSize != test.want {
				t.Fatalf("BufferSize = %d, want %d", got.BufferSize, test.want)
			}
			if got.BitRateNominal != int64((bitRateHigh<<3)+bitRateLow)*400 {
				t.Fatalf("neighboring BitRateNominal = %d", got.BitRateNominal)
			}
		})
	}
}

func TestMPEG4VisualBufferSizeProjectsAcrossRenderers(t *testing.T) {
	const raw = int64(75_063_953_408)
	general := Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "AVI"}}}
	video := Stream{
		Kind: StreamVideo,
		Fields: []Field{
			{Name: "Format", Value: "MPEG-4 Visual"},
			{Name: "Buffer size", Value: "75.1 Gb"},
		},
		JSON: map[string]string{"BufferSize": strconv.FormatInt(raw, 10)},
	}
	report := Report{General: general, Streams: []Stream{video}}
	for name, output := range map[string]string{
		"JSON": RenderJSON([]Report{report}),
		"XML":  RenderXML([]Report{report}),
		"text": RenderText([]Report{report}),
		"CSV":  RenderCSV([]Report{report}),
		"HTML": RenderHTML([]Report{report}),
	} {
		if name == "JSON" || name == "XML" {
			if !strings.Contains(output, strconv.FormatInt(raw, 10)) {
				t.Fatalf("%s omitted raw BufferSize: %s", name, output)
			}
			continue
		}
		if !strings.Contains(output, "75.1 Gb") {
			t.Fatalf("%s omitted displayed BufferSize: %s", name, output)
		}
	}
}

func buildMPEG4VOLWithVBV(bitRateHigh, bitRateLow, bufferHigh, bufferLow uint32) []byte {
	data := make([]byte, 32)
	pos := 0
	writeBits(data, &pos, 0, 1) // random_accessible_vol
	writeBits(data, &pos, 1, 8) // video_object_type_indication
	writeBits(data, &pos, 0, 1) // is_object_layer_identifier
	writeBits(data, &pos, 1, 4) // square pixels
	writeBits(data, &pos, 1, 1) // vol_control_parameters
	writeBits(data, &pos, 1, 2) // 4:2:0
	writeBits(data, &pos, 0, 1) // low_delay
	writeBits(data, &pos, 1, 1) // vbv_parameters
	writeBits(data, &pos, bitRateHigh, 15)
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, bitRateLow, 15)
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, bufferHigh, 15)
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, bufferLow, 3)
	writeBits(data, &pos, 0, 11)
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, 0, 15)
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, 0, 2)     // rectangular shape
	writeBits(data, &pos, 1, 1)     // marker
	writeBits(data, &pos, 1000, 16) // vop_time_increment_resolution
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, 0, 1)    // fixed_vop_rate
	writeBits(data, &pos, 1, 1)    // marker
	writeBits(data, &pos, 640, 13) // width
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, 360, 13) // height
	writeBits(data, &pos, 1, 1)
	writeBits(data, &pos, 0, 1) // progressive
	writeBits(data, &pos, 1, 1) // obmc_disable
	writeBits(data, &pos, 0, 1) // sprite_enable (verid 1)
	writeBits(data, &pos, 0, 1) // not_8_bit
	writeBits(data, &pos, 0, 1) // quant_type
	return data[:(pos+7)/8]
}
