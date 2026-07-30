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
	if method := h264FieldPictureStoreMethod(makeFieldSlice(false), sps); method != "SeparatedFields" {
		t.Fatalf("field picture store method = %q, want SeparatedFields", method)
	}
	if method := h264FieldPictureStoreMethod(makeFieldSlice(false), h264SPSInfo{Log2MaxFrameNumMinus4: 0, MBAFF: true}); method != "" {
		t.Fatalf("MBAFF field picture store method = %q, want empty", method)
	}
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

func TestH264DisplayOrderRejectsZeroValueSeededSPS(t *testing.T) {
	slice := make([]byte, 2)
	writer := bitWriter{b: slice}
	writer.writeBits(1, 1) // first_mb_in_slice = 0
	writer.writeBits(3, 3) // slice_type = 2 (I)
	writer.writeBits(1, 1) // pic_parameter_set_id = 0
	writer.writeBits(0, 4) // frame_num
	writer.writeBits(0, 1) // field_pic_flag
	writer.writeBits(1, 1) // idr_pic_id = 0
	writer.writeBits(0, 4) // pic_order_cnt_lsb
	payload := append([]byte{0, 0, 1, 0x65}, slice...)

	if got := h264DisplayOrderPictureTypes(payload, 1, h264SPSInfo{}); len(got) != 0 {
		t.Fatalf("zero-value seeded SPS produced display order %q", got)
	}
}

func TestH264ExpGolombRejectsThirtyTwoLeadingZeros(t *testing.T) {
	br := newBitReader(make([]byte, 5))
	if value, ok := br.readUEWithOk(); ok {
		t.Fatalf("32-zero Exp-Golomb accepted with value %d", value)
	}
}

func TestH264GOPPendingBufferIsBounded(t *testing.T) {
	chunk := make([]byte, maxH264GOPPendingBytes+16)
	copy(chunk, []byte{0, 0, 1, 0x41})
	_, pending := appendH264PictureTypes(nil, nil, chunk, 8, nil, nil, nil)
	if len(pending) > 3 {
		t.Fatalf("pending AVC bytes = %d, want bounded tail", len(pending))
	}
}
