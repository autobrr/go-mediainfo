package mediainfo

import "testing"

// TestBuildHEVCFieldsFromSPS_TierAndChromaLoc pins that the Annex-B (MPEG-TS/BDAV)
// HEVC path matches the shared parseHEVCConfig behavior: Format tier is reported for
// Main as well as High, and Chroma subsampling position is emitted only when the SPS
// signals chroma_loc_info (mirroring chroma_sample_loc_type_top_field), not hardcoded
// "Type 2" for every 4:2:0 stream.
func TestBuildHEVCFieldsFromSPS_TierAndChromaLoc(t *testing.T) {
	find := func(fs []Field, name string) (string, bool) {
		for _, f := range fs {
			if f.Name == name {
				return f.Value, true
			}
		}
		return "", false
	}

	t.Run("Main tier emitted", func(t *testing.T) {
		f := buildHEVCFieldsFromSPS(h264SPSInfo{ProfileID: 1, LevelID: 120, HEVCTier: "Main", ChromaFormat: "4:2:0"})
		if v, ok := find(f, "Format tier"); !ok || v != "Main" {
			t.Errorf("Format tier = %q (present=%v), want Main", v, ok)
		}
	})

	t.Run("High tier still emitted", func(t *testing.T) {
		f := buildHEVCFieldsFromSPS(h264SPSInfo{ProfileID: 1, LevelID: 120, HEVCTier: "High", ChromaFormat: "4:2:0"})
		if v, ok := find(f, "Format tier"); !ok || v != "High" {
			t.Errorf("Format tier = %q (present=%v), want High", v, ok)
		}
	})

	t.Run("chroma position only when signaled", func(t *testing.T) {
		// 4:2:0 but chroma_loc not signaled -> no position (was wrongly "Type 2").
		f := buildHEVCFieldsFromSPS(h264SPSInfo{ProfileID: 1, HEVCTier: "Main", ChromaFormat: "4:2:0", HasChromaLoc: false, ChromaSampleLoc: -1})
		if v, ok := find(f, "Chroma subsampling position"); ok {
			t.Errorf("Chroma subsampling position = %q, want unset (chroma_loc not signaled)", v)
		}
	})

	t.Run("chroma position mirrors signaled value", func(t *testing.T) {
		f := buildHEVCFieldsFromSPS(h264SPSInfo{ProfileID: 1, HEVCTier: "Main", ChromaFormat: "4:2:0", HasChromaLoc: true, ChromaSampleLoc: 0})
		if v, ok := find(f, "Chroma subsampling position"); !ok || v != "Type 0" {
			t.Errorf("Chroma subsampling position = %q (present=%v), want Type 0", v, ok)
		}
	})
}

func TestMergeTSHEVCMasteringMetadataPreservesZeroLuminance(t *testing.T) {
	destination := hevcHDRInfo{}
	source := hevcHDRInfo{
		masteringPrimaries:    "Display P3",
		masteringLuminanceMin: 0,
		masteringLuminanceMax: 0,
		hasMastering:          true,
	}
	mergeTSHEVCMasteringMetadata(&destination, source)
	if !destination.hasMastering {
		t.Fatal("mastering metadata presence was lost")
	}
	if destination.masteringLuminanceMin != 0 || destination.masteringLuminanceMax != 0 {
		t.Fatalf("luminance = %v/%v, want 0/0", destination.masteringLuminanceMin, destination.masteringLuminanceMax)
	}
	if destination.masteringPrimaries != source.masteringPrimaries {
		t.Fatalf("primaries = %q, want %q", destination.masteringPrimaries, source.masteringPrimaries)
	}
	if got := formatMasteringLuminance(0, 0); got != "min: 0.0000 cd/m2, max: 0 cd/m2" {
		t.Fatalf("formatted luminance = %q", got)
	}
}
