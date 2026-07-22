package mediainfo

import "testing"

func TestH264FirstFieldOrderUsesSliceFlags(t *testing.T) {
	makeFieldSlice := func(bottom bool) []byte {
		payload := make([]byte, 2)
		writer := bitWriter{b: payload}
		writer.writeBits(1, 1) // first_mb_in_slice = 0
		writer.writeBits(1, 1) // slice_type = 0
		writer.writeBits(1, 1) // pic_parameter_set_id = 0
		writer.writeBits(0, 4) // frame_num
		writer.writeBits(1, 1) // field_pic_flag
		if bottom {
			writer.writeBits(1, 1)
		} else {
			writer.writeBits(0, 1)
		}
		return append([]byte{0, 0, 1, 0x41}, payload...)
	}

	sps := h264SPSInfo{Log2MaxFrameNumMinus4: 0}
	for _, test := range []struct {
		name   string
		bottom bool
		want   string
	}{
		{name: "top", want: "TFF"},
		{name: "bottom", bottom: true, want: "BFF"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := h264FirstFieldOrder(makeFieldSlice(test.bottom), sps)
			if !ok || got != test.want {
				t.Fatalf("field order = %q, %v; want %q", got, ok, test.want)
			}
		})
	}
}
