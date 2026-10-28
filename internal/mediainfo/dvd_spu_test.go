package mediainfo

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"testing"
)

// testDVDSPU contains a two-line pixel region and bounded display commands.
// It tests structural acceptance, not run-length bitmap decoding.
func testDVDSPU() []byte {
	return []byte{0, 24, 0, 6, 0x90, 0x90, 0, 0, 0, 6, 1, 5, 0, 0, 1, 0, 0, 1, 6, 0, 4, 0, 5, 0xff}
}

func testDVDSPUPES(id byte, payload []byte) []byte {
	packet := make([]byte, 0, 10+len(payload))
	packet = append(packet, 0, 0, 1, 0xbd, 0, 0, 0x80, 0, 0, id)
	binary.BigEndian.PutUint16(packet[4:6], uint16(4+len(payload)))
	return append(packet, payload...)
}

func TestSupplementalDVDSubpictureAcceptance(t *testing.T) {
	valid := testDVDSPU()
	for _, tc := range []struct {
		name      string
		fragments [][]byte
		want      bool
	}{
		{"complete", [][]byte{valid}, true},
		{"split PES", [][]byte{valid[:12], valid[12:]}, true},
		{"split header", [][]byte{valid[:1], valid[1:3], valid[3:]}, true},
		{"header only", [][]byte{nil}, false},
		{"arbitrary byte", [][]byte{{0}}, false},
		{"truncated unit", [][]byte{valid[:23]}, false},
		{"invalid size", [][]byte{{0, 3, 0, 6}}, false},
		{"bad control offset", [][]byte{append([]byte{0, 24, 0, 25}, valid[4:]...)}, false},
		{"bad pixel offset", [][]byte{append(append([]byte{}, valid[:19]...), 0, 6, 0, 5, 0xff)}, false},
		{"missing terminator", [][]byte{append(append([]byte{}, valid[:23]...), 1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data []byte
			for _, fragment := range tc.fragments {
				data = append(data, testDVDSPUPES(0x20, fragment)...)
			}
			streams := appendDVDPayloadSubpictures(bytes.NewReader(data), int64(len(data)), mpegPSOptions{dvdParsing: true, parseSpeed: 0.5}, nil)
			if (len(streams) > 0) != tc.want {
				t.Fatalf("accepted %d streams, want acceptance %v", len(streams), tc.want)
			}
		})
	}
}

func TestSupplementalDVDSubpictureDiscontinuity(t *testing.T) {
	data := make([]byte, 48<<20)
	valid := testDVDSPU()
	copy(data, testDVDSPUPES(0x20, valid[:12]))
	tail := testDVDSPUPES(0x20, valid[12:])
	copy(data[len(data)-len(tail):], tail)
	streams := appendDVDPayloadSubpictures(bytes.NewReader(data), int64(len(data)), mpegPSOptions{dvdParsing: true, parseSpeed: 0.5}, nil)
	if len(streams) != 0 {
		t.Fatal("assembled an SPU across a sampled gap")
	}
}

func TestSupplementalDVDSubpictureShortUnitRecovery(t *testing.T) {
	for size := 2; size < 10; size++ {
		short := bytes.Repeat([]byte{0xa5}, size)
		binary.BigEndian.PutUint16(short, uint16(size))
		for _, tc := range []struct {
			name      string
			fragments [][]byte
			want      bool
		}{
			{"same PES", [][]byte{append(append([]byte{}, short...), testDVDSPU()...)}, true},
			{"split PES", [][]byte{short[:1], short[1 : size-1], append(append([]byte{}, short[size-1:]...), testDVDSPU()...)}, true},
			{"short only", [][]byte{short}, false},
		} {
			t.Run(strconv.Itoa(size)+"/"+tc.name, func(t *testing.T) {
				var data []byte
				for _, fragment := range tc.fragments {
					data = append(data, testDVDSPUPES(0x20, fragment)...)
				}
				streams := appendDVDPayloadSubpictures(bytes.NewReader(data), int64(len(data)), mpegPSOptions{dvdParsing: true, parseSpeed: 0.5}, nil)
				if !tc.want {
					if len(streams) != 0 {
						t.Fatal("short unit admitted a subtitle stream")
					}
					return
				}
				if len(streams) != 1 {
					t.Fatalf("accepted %d streams, want the valid trailing SPU", len(streams))
				}
				if id, _ := canonicalSeedValue(streams[0], "ID"); id != "189-32" {
					t.Fatalf("subtitle ID = %q, want 189-32", id)
				}
			})
		}
	}
}

func TestDVDSPUProbeImpossibleSize(t *testing.T) {
	for _, size := range []byte{0, 1} {
		t.Run(strconv.Itoa(int(size)), func(t *testing.T) {
			probe := dvdSPUProbe{}
			probe.consume(append([]byte{0, size}, testDVDSPU()...), 0, false)
			if probe.accepted || len(probe.pending) != 0 {
				t.Fatal("impossible size did not discard the current payload")
			}
			probe.consume(testDVDSPU(), 0, false)
			if !probe.accepted {
				t.Fatal("next payload did not recover after an impossible size")
			}
		})
	}
}

func TestDVDSPUProbeShortUnitSectionReset(t *testing.T) {
	parser := newPSStreamParser(mpegPSOptions{})
	parser.dvdSubpictures = map[uint16]*dvdSPUProbe{}
	key := psStreamKey(0xbd, 0x20)
	entry := parser.ensureStream(0xbd, 0x20, StreamText, "RLE")
	parser.consumePayload(entry, key, 0, 1, true, []byte{0, 9, 0xa5})
	parser.beginSection()
	parser.consumePayload(entry, key, 0, 1, true, testDVDSPU())
	if !parser.dvdSubpictures[key].accepted {
		t.Fatal("sampled gap retained a partial rejected unit")
	}
}

func TestDVDSPUProbeTimestampRecovery(t *testing.T) {
	valid := testDVDSPU()
	split := dvdSPUProbe{}
	split.consume(valid[:12], 1, true)
	split.consume(valid[12:], 1, true)
	if !split.accepted {
		t.Fatal("repeated timestamp discarded a valid continuation")
	}
	probe := dvdSPUProbe{}
	probe.consume([]byte{0xff, 0xff, 0, 4}, 1, true)
	probe.consume(testDVDSPU(), 2, true)
	if !probe.accepted {
		t.Fatal("new timestamp did not abandon incomplete unit")
	}
	short := dvdSPUProbe{}
	short.consume([]byte{0, 9, 0xa5}, 1, true)
	short.consume(testDVDSPU(), 2, true)
	if !short.accepted {
		t.Fatal("new timestamp retained a partial rejected unit")
	}
}

func TestGeneralPSRetainsRLEIdentityContract(t *testing.T) {
	data := testDVDSPUPES(0x20, []byte{0})
	_, streams, ok := ParseMPEGPSWithOptions(bytes.NewReader(data), int64(len(data)), mpegPSOptions{})
	if !ok || len(streams) != 1 {
		t.Fatal("supplemental gate changed general PES identity contract")
	}
}

func TestDVDSPUControlChain(t *testing.T) {
	chained := append(testDVDSPU(), 0, 1, 0, 24, 2, 0xff)
	binary.BigEndian.PutUint16(chained[:2], uint16(len(chained)))
	binary.BigEndian.PutUint16(chained[8:10], 24)
	if !validDVDSPU(chained) {
		t.Fatal("valid terminal control sequence rejected")
	}
	binary.BigEndian.PutUint16(chained[26:28], 6)
	if validDVDSPU(chained) {
		t.Fatal("backward control chain accepted")
	}
	variable := append(append([]byte{}, testDVDSPU()[:23]...), 7, 0, 6, 0, 0, 0, 0, 0xff)
	binary.BigEndian.PutUint16(variable[:2], uint16(len(variable)))
	if !validDVDSPU(variable) {
		t.Fatal("bounded color/contrast command rejected")
	}
	variable[10] = 2 // Color/contrast starts display without a separate start command.
	if !validDVDSPU(variable) {
		t.Fatal("color/contrast display command requires unrelated start command")
	}
	variable[25] = 255
	if validDVDSPU(variable) {
		t.Fatal("out-of-range variable command accepted")
	}
}
