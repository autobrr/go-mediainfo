package mediainfo

import "testing"

// aacTestBitWriter assembles bit-exact AAC test payloads.
type aacTestBitWriter struct {
	bits []byte
}

func (w *aacTestBitWriter) put(value uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, byte(value>>uint(i)&1))
	}
}

func (w *aacTestBitWriter) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, b := range w.bits {
		if b != 0 {
			out[i>>3] |= 1 << (7 - uint(i&7))
		}
	}
	return out
}

func TestParseAACRDBConfigLC(t *testing.T) {
	// Standard AAC LC 48 kHz stereo AudioSpecificConfig.
	cfg, ok := parseAACRDBConfig([]byte{0x11, 0x90})
	if !ok {
		t.Fatal("expected config parse to succeed")
	}
	if cfg.objectType != 2 || cfg.samplingFrequencyIndex != 3 || cfg.frameLength != 1024 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !cfg.walkable() {
		t.Fatal("expected LC config to be walkable")
	}
}

func TestParseAACRDBConfigExplicitSBRKeepsCoreRate(t *testing.T) {
	// Hierarchical HE-AAC signaling: audioObjectType 5 (SBR), core rate
	// 24 kHz (index 6), extension rate 48 kHz (index 3), core object type 2.
	// MediaInfoLib keeps sampling_frequency_index at the core index when the
	// extension index is not escaped.
	w := &aacTestBitWriter{}
	w.put(5, 5) // audioObjectType = SBR
	w.put(6, 4) // samplingFrequencyIndex = 24 kHz
	w.put(2, 4) // channelConfiguration
	w.put(3, 4) // extensionSamplingFrequencyIndex = 48 kHz
	w.put(2, 5) // audioObjectType = LC
	w.put(0, 3) // GASpecificConfig flags
	cfg, ok := parseAACRDBConfig(w.bytes())
	if !ok {
		t.Fatal("expected config parse to succeed")
	}
	if cfg.objectType != 2 || cfg.samplingFrequencyIndex != 6 || cfg.frameLength != 1024 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !cfg.walkable() {
		t.Fatal("expected HE-AAC core config to be walkable")
	}
}

// aacTestSCE writes a minimal single_channel_element with max_sfb 0 (no
// sections, no scale factors, no spectral data).
func aacTestSCE(w *aacTestBitWriter) {
	w.put(0, 3) // id_syn_ele = ID_SCE
	w.put(0, 4) // element_instance_tag
	w.put(0, 8) // global_gain
	w.put(0, 1) // ics_reserved_bit
	w.put(0, 2) // window_sequence = ONLY_LONG_SEQUENCE
	w.put(0, 1) // window_shape
	w.put(0, 6) // max_sfb = 0
	w.put(0, 1) // predictor_data_present
	w.put(0, 1) // pulse_data_present
	w.put(0, 1) // tns_data_present
	w.put(0, 1) // gain_control_data_present
}

func TestAACWalkFrameFindsEnd(t *testing.T) {
	cfg, ok := parseAACRDBConfig([]byte{0x11, 0x90})
	if !ok {
		t.Fatal("config parse failed")
	}
	state := newAACRDBState(cfg)
	w := &aacTestBitWriter{}
	aacTestSCE(w)
	w.put(7, 3) // ID_END
	if !state.walkFrame(w.bytes()) {
		t.Fatal("expected walk to find ID_END")
	}
}

func TestAACWalkFrameMissingEnd(t *testing.T) {
	cfg, ok := parseAACRDBConfig([]byte{0x11, 0x90})
	if !ok {
		t.Fatal("config parse failed")
	}
	state := newAACRDBState(cfg)
	// All-zero payload: walks a zero SCE, then runs out of bits while reading
	// the next element without ever seeing ID_END.
	if state.walkFrame([]byte{0x00, 0x00, 0x00, 0x00}) {
		t.Fatal("expected walk to miss ID_END")
	}
	// State survives a failed frame; a well-formed frame still walks.
	w := &aacTestBitWriter{}
	aacTestSCE(w)
	w.put(7, 3)
	if !state.walkFrame(w.bytes()) {
		t.Fatal("expected walk to find ID_END after a failed frame")
	}
}

func TestAACWalkFrameFillElement(t *testing.T) {
	cfg, ok := parseAACRDBConfig([]byte{0x11, 0x90})
	if !ok {
		t.Fatal("config parse failed")
	}
	state := newAACRDBState(cfg)
	w := &aacTestBitWriter{}
	aacTestSCE(w)
	w.put(6, 3) // ID_FIL
	w.put(2, 4) // count = 2 bytes
	w.put(0xA5A5, 16)
	w.put(7, 3) // ID_END
	if !state.walkFrame(w.bytes()) {
		t.Fatal("expected walk to skip fill element and find ID_END")
	}
	// Fill element whose count exceeds the remaining payload consumes the
	// frame without ID_END.
	w = &aacTestBitWriter{}
	aacTestSCE(w)
	w.put(6, 3)  // ID_FIL
	w.put(14, 4) // count = 14 bytes, more than remains
	w.put(0, 8)
	if state.walkFrame(w.bytes()) {
		t.Fatal("expected oversized fill element to miss ID_END")
	}
}

func TestProbeMatroskaAudioAACDispatch(t *testing.T) {
	cfg, ok := parseAACRDBConfig([]byte{0x11, 0x90})
	if !ok {
		t.Fatal("config parse failed")
	}
	good := &aacTestBitWriter{}
	aacTestSCE(good)
	good.put(7, 3)
	goodFrame := good.bytes()

	probe := &matroskaAudioProbe{format: "AAC", aacState: newAACRDBState(cfg), aacTargetFrames: 2}
	probes := map[uint64]*matroskaAudioProbe{1: probe}
	probeMatroskaAudio(probes, 1, goodFrame, 1, int64(len(goodFrame)), true)
	if probe.aacMissingEnd || probe.ok || probe.aacFrames != 1 {
		t.Fatalf("after clean frame: %+v", probe)
	}
	// A truncated peek must not be evaluated.
	probeMatroskaAudio(probes, 1, goodFrame[:2], 1, int64(len(goodFrame)), true)
	if probe.aacFrames != 1 {
		t.Fatalf("truncated frame was counted: %+v", probe)
	}
	probeMatroskaAudio(probes, 1, []byte{0x00, 0x00, 0x00, 0x00}, 1, 4, true)
	if !probe.aacMissingEnd || !probe.ok || !probe.exhausted {
		t.Fatalf("missing END not flagged: %+v", probe)
	}
}
