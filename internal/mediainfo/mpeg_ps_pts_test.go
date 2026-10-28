package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMPEGPSRLEDurationEvidence(t *testing.T) {
	const wrap = uint64(1 << 33)
	for _, tc := range []struct {
		name     string
		pts      []uint64
		gap      bool
		sampled  bool
		noPTS    bool
		generic  bool
		duration string
	}{
		{name: "monotonic", pts: []uint64{90_000, 180_000}, duration: "1000"},
		{name: "small reorder", pts: []uint64{90_000, 180_000, 170_000}, duration: "1000"},
		{name: "continuous wrap", pts: []uint64{wrap - 90_000, 90_000}, duration: "2000"},
		{name: "wrap with forward endpoint", pts: []uint64{90_000, wrap - 90_000, 0, 180_000}},
		{name: "two observed wraps", pts: []uint64{wrap - 90_000, 90_000, wrap - 90_000, 90_000}},
		{name: "forward step over half range", pts: []uint64{90_000, wrap - 90_000}},
		{name: "forward step at half range", pts: []uint64{90_000, 90_000 + wrap/2}},
		{name: "forward half range after reorder", pts: []uint64{180_000, 90_000, 90_000 + wrap/2}},
		{name: "backward half range after reorder", pts: []uint64{wrap/2 + 90_000, wrap / 2, 0}},
		{name: "generic backward reset", pts: []uint64{900_000, 90_000}, generic: true},
		{name: "generic ambiguous forward step", pts: []uint64{90_000, wrap - 90_000, 0, 180_000}, generic: true},
		{name: "wrap within sampled section", pts: []uint64{wrap - 90_000, 90_000}, sampled: true, duration: "2000"},
		{name: "sampled monotonic span", pts: []uint64{90_000, 180_000}, gap: true, sampled: true, duration: "1000"},
		{name: "backward reset", pts: []uint64{900_000, 90_000}},
		{name: "forward endpoint after reset", pts: []uint64{90_000, 900_000, 0, 180_000}},
		{name: "sampled reset endpoints", pts: []uint64{1_211_442, 44_722}, gap: true, sampled: true},
		{name: "wrap across unsampled gap", pts: []uint64{wrap - 90_000, 90_000}, gap: true, sampled: true},
		{name: "reset then wrap", pts: []uint64{900_000, 0, wrap - 90_000, 90_000}},
		{name: "single timestamp", pts: []uint64{90_000}},
		{name: "no timestamps", noPTS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := newPSStreamParser(mpegPSOptions{})
			if tc.noPTS {
				if !parser.parseReader(bytes.NewReader(testDVDSPUPES(0x21, testDVDSPU()))) {
					t.Fatal("payload did not parse")
				}
			}
			for i, pts := range tc.pts {
				if tc.gap && i == 1 {
					parser.beginSection()
				}
				packet := testMPEGPSPTS(0xbd, pts, append([]byte{0x21}, testDVDSPU()...))
				if !parser.parseReader(bytes.NewReader(packet)) {
					t.Fatal("timestamped payload did not parse")
				}
			}
			var videoPTS ptsTracker
			videoPTS.add(0)
			info, streams, ok := finalizeMPEGPS(parser.streams, parser.streamOrder, nil, videoPTS, parser.anyPTS, 4096,
				mpegPSOptions{dvdParsing: !tc.generic, parseSpeed: 0.5, sampled: tc.sampled})
			if !ok || len(streams) != 1 {
				t.Fatalf("finalization lost payload stream: ok=%v streams=%d", ok, len(streams))
			}
			stream := streams[0]
			if id, _ := canonicalSeedValue(stream, "ID"); id != "189-33" {
				t.Fatalf("ID = %q", id)
			}
			if got, present := canonicalSeedValue(stream, "Duration"); got != tc.duration || present != (tc.duration != "") {
				t.Errorf("Duration = %q, present=%v; want %q", got, present, tc.duration)
			}
			if tc.duration == "" && findField(stream.Fields, "Duration") != "" {
				t.Error("unsupported duration leaked into text fields")
			}
			if tc.duration == "" && info.DurationSeconds != 0 {
				t.Errorf("unsupported container duration = %.12f", info.DurationSeconds)
			}
			if len(tc.pts) > 0 {
				want := float64(tc.pts[0]) / 90_000
				for _, key := range []fieldName{"Delay", "Video_Delay"} {
					value, present := canonicalSeedValue(stream, key)
					got, valid := parseFloatValue(value)
					if !present || !valid || math.Abs(got-want) > 0.0005 {
						t.Errorf("%s = %q; want preserved first PTS %.9f", key, value, want)
					}
				}
			}
		})
	}
}

func TestMPEGPSContainerRLEDuration(t *testing.T) {
	const wrap = uint64(1 << 33)
	video := append([]byte{0, 0, 1, 0xb3}, makeMPEG2SequenceHeaderWithoutCustomMatrices()...)
	video = append(video, 0, 0, 1, 0, 0, 0x0f, 0xff, 0xf8)
	audio := append([]byte{0x80, 0, 0, 0}, buildAC3FrameForMatroskaProbe()...)
	av := makeMPEGPSPackHeader(true, 20000)
	for _, pts := range []uint64{90_000, 180_000} {
		av = append(av, testMPEGPSPTS(0xe0, pts, video)...)
		av = append(av, testMPEGPSPTS(0xbd, pts, audio)...)
	}
	for _, tc := range []struct {
		name                      string
		pts                       []uint64
		mixed, validRLE, splitIDs bool
		untimedVideo              bool
		want                      float64
	}{
		{name: "invalid only", pts: []uint64{90_000, wrap - 90_000}},
		{name: "invalid with AV", pts: []uint64{90_000, wrap - 90_000}, mixed: true, want: 1},
		{name: "valid with AV", pts: []uint64{90_000, 180_000}, mixed: true, want: 1},
		{name: "invalid with valid RLE", pts: []uint64{90_000, wrap - 90_000}, validRLE: true, want: 1},
		{name: "invalid with untimed video header", pts: []uint64{90_000, wrap - 90_000}, untimedVideo: true},
		{name: "valid with untimed video header", pts: []uint64{90_000, 180_000}, untimedVideo: true, want: 1 + 1/29.97},
		{name: "single PTS", pts: []uint64{90_000}},
		{name: "unflagged cross track fallback", pts: []uint64{90_000, wrap - 90_000}, splitIDs: true, want: float64(wrap-180_000) / 90_000},
	} {
		for _, dvd := range []bool{false, true} {
			mode := "generic"
			if dvd {
				mode = "dvd"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				opts := mpegPSOptions{dvdParsing: dvd, parseSpeed: 0.5}
				data := append([]byte{}, makeMPEGPSPackHeader(true, 20000)...)
				if tc.untimedVideo {
					packet := append([]byte{0, 0, 1, 0xe0, 0, 0, 0x80, 0, 0}, video[:12]...)
					packet = append(packet, 0, 0, 1, 0xb7)
					binary.BigEndian.PutUint16(packet[4:6], uint16(len(packet)-6))
					data = append(data, packet...)
				}
				var control []Stream
				if tc.mixed {
					data = append([]byte{}, av...)
					// Equal-sized control packets isolate timing from DVD byte accounting.
					controlData := append([]byte{}, av...)
					for _, pts := range []uint64{90_000, 180_000} {
						controlData = append(controlData, testMPEGPSPTS(0xbd, pts, append([]byte{0x21}, testDVDSPU()...))...)
					}
					_, control, _ = ParseMPEGPSWithOptions(bytes.NewReader(controlData), int64(len(controlData)), opts)
					if len(control) != 3 {
						t.Fatal("synthetic A/V control did not parse")
					}
				}
				for i, pts := range tc.pts {
					id := byte(0x21)
					if tc.splitIDs {
						id += byte(i)
					}
					data = append(data, testMPEGPSPTS(0xbd, pts, append([]byte{id}, testDVDSPU()...))...)
				}
				if tc.validRLE {
					for _, pts := range []uint64{270_000, 360_000} {
						data = append(data, testMPEGPSPTS(0xbd, pts, append([]byte{0x22}, testDVDSPU()...))...)
					}
				}
				info, streams, ok := ParseMPEGPSWithOptions(bytes.NewReader(data), int64(len(data)), opts)
				if tc.untimedVideo {
					t.Logf("untimed-video container seconds=%.12f", info.DurationSeconds)
					for _, stream := range streams {
						if stream.Kind == StreamVideo {
							duration, present := canonicalSeedValue(stream, "Duration")
							frames, hasFrames := canonicalSeedValue(stream, "FrameCount")
							t.Logf("video Duration-ms=%s present=%v frames=%s", duration, present, frames)
							if tc.want == 0 && (present || hasFrames) {
								t.Error("unsupported video duration/frame count survived")
							}
							for field, want := range map[fieldName]string{"ID": "224", "Width": "720", "Height": "480"} {
								if got, _ := canonicalSeedValue(stream, field); got != want {
									t.Errorf("video %s = %q, want %q", field, got, want)
								}
							}
							rate, _ := canonicalSeedValue(stream, "FrameRate")
							if value, valid := parseFloatValue(rate); !valid || math.Abs(value-29.97) > 0.0001 {
								t.Errorf("video frame rate changed: %q", rate)
							}
						}
					}
				}
				if !ok || math.Abs(info.DurationSeconds-tc.want) > 1e-9 {
					t.Errorf("container seconds = %.12f, ok=%v; want %.12f", info.DurationSeconds, ok, tc.want)
				}
				for _, before := range control {
					if before.Kind != StreamAudio && before.Kind != StreamVideo {
						continue
					}
					id, _ := canonicalSeedValue(before, "ID")
					found := false
					for _, after := range streams {
						if candidate, _ := canonicalSeedValue(after, "ID"); candidate != id {
							continue
						}
						found = true
						for _, field := range []fieldName{"Duration", "FrameCount", "SamplingCount", "SamplingRate", "BitRate"} {
							value, present := canonicalSeedValue(before, field)
							if got, found := canonicalSeedValue(after, field); got != value || found != present {
								t.Errorf("A/V %s %s changed: %q/%v to %q/%v", id, field, value, present, got, found)
							}
						}
					}
					if !found {
						t.Errorf("missing A/V stream %s", id)
					}
				}
				if tc.validRLE {
					found := false
					for _, stream := range streams {
						if id, _ := canonicalSeedValue(stream, "ID"); id == "189-34" {
							found = true
							if duration, _ := canonicalSeedValue(stream, "Duration"); duration != "1000" {
								t.Errorf("valid RLE duration = %q", duration)
							}
						}
					}
					if !found {
						t.Error("valid RLE stream was lost")
					}
				}
				// Unknown duration must omit its derived bitrate rather than divide by zero.
				if tc.want == 0 {
					root, name := t.TempDir(), "synthetic.mpg"
					if dvd {
						root, name = filepath.Join(root, "VIDEO_TS"), "VTS_01_1.VOB"
						if err := os.Mkdir(root, 0o700); err != nil {
							t.Fatal(err)
						}
					}
					path := filepath.Join(root, name)
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
					report, err := AnalyzeFileWithOptions(path, AnalyzeOptions{ParseSpeed: 0.5})
					if err != nil {
						t.Fatal(err)
					}
					var output struct {
						Media struct {
							Track []map[string]any `json:"track"`
						} `json:"media"`
					}
					if err := json.Unmarshal([]byte(RenderJSON([]Report{report})), &output); err != nil {
						t.Fatal(err)
					}
					for _, track := range output.Media.Track {
						if track["@type"] == "General" {
							for _, field := range []string{"Duration", "OverallBitRate"} {
								if value, found := track[field]; found {
									t.Errorf("unknown General %s = %v", field, value)
								}
							}
						}
					}
				}
			})
		}
	}
}

// testMPEGPSPTS wraps a payload in a bounded PES with a 33-bit timestamp.
func testMPEGPSPTS(id byte, pts uint64, payload []byte) []byte {
	packet := []byte{0, 0, 1, id, 0, 0, 0x80, 0x80, 5,
		0x21 | byte(pts>>30&7)<<1, byte(pts >> 22), byte(pts>>15&0x7f)<<1 | 1,
		byte(pts >> 7), byte(pts&0x7f)<<1 | 1}
	packet = append(packet, payload...)
	binary.BigEndian.PutUint16(packet[4:6], uint16(len(packet)-6))
	return packet
}

func TestPtsDurationPS_Resets(t *testing.T) {
	var tracker ptsTracker
	tracker.add(0)
	tracker.add(90_000 * 100) // 100 seconds
	tracker.add(0)            // discontinuity
	tracker.add(90_000 * 10)  // 10 seconds

	t.Run("parseSpeed<1 uses total", func(t *testing.T) {
		got := ptsDurationPS(tracker, mpegPSOptions{parseSpeed: 0.5})
		if got != 110 {
			t.Fatalf("duration=%v, want 110", got)
		}
	})

	t.Run("parseSpeed>=1 uses minmax span (non-dvd)", func(t *testing.T) {
		got := ptsDurationPS(tracker, mpegPSOptions{parseSpeed: 1})
		if got != 100 {
			t.Fatalf("duration=%v, want 100", got)
		}
	})

	t.Run("dvdParsing uses last segment", func(t *testing.T) {
		got := ptsDurationPS(tracker, mpegPSOptions{parseSpeed: 1, dvdParsing: true})
		if got != 10 {
			t.Fatalf("duration=%v, want 10", got)
		}
	})
}

func TestSampledFrameClockDurationPSUsesResetSegmentStart(t *testing.T) {
	var tracker ptsTracker
	tracker.add(9_000_000)
	tracker.add(9_090_000)
	tracker.add(90_000)
	tracker.add(180_000)
	if !tracker.hasResets() || tracker.segmentStart == tracker.first {
		t.Fatalf("tracker did not establish distinct reset segment: %#v", tracker)
	}
	stream := &psStream{
		pts:             tracker,
		audioFrames:     12,
		sampleSection:   2,
		clockPTS:        270_000,
		clockHasPTS:     true,
		clockAudioStart: 10,
	}
	got, ok := sampledFrameClockDurationPS(stream, mpegPSOptions{sampled: true}, 48_000, 1152, false)
	want := 2.0 + 2.0*1152.0/48_000.0
	if !ok || math.Abs(got-want) > 1e-9 {
		t.Fatalf("sampledFrameClockDurationPS() = %.9f, %v; want %.9f, true", got, ok, want)
	}
}
