package mediainfo

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"testing"
)

type sparseReadSeeker struct {
	data    []byte
	size    int64
	pos     int64
	maxRead int
}

func (r *sparseReadSeeker) Read(p []byte) (int, error) {
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	if r.pos < 0 || r.pos >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += int64(n)
	if n != len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *sparseReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = r.pos + offset
	case io.SeekEnd:
		next = r.size + offset
	default:
		return 0, io.ErrUnexpectedEOF
	}
	if next < 0 {
		return 0, io.ErrUnexpectedEOF
	}
	r.pos = next
	return next, nil
}

func TestParseFLACPictureRejectsOverflowingDescriptionLength(t *testing.T) {
	data := make([]byte, 32)
	binary.BigEndian.PutUint32(data[0:4], 3)
	binary.BigEndian.PutUint32(data[4:8], 0)
	binary.BigEndian.PutUint32(data[8:12], math.MaxUint32)
	if mime, typ, ok := parseFLACPicture(data); ok {
		t.Fatalf("overflowing picture parsed as mime=%q type=%q", mime, typ)
	}
}

func TestParseFLACLargePictureReadsOnlyBoundedHeader(t *testing.T) {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		t.Fatal(err)
	}
	const pictureBlockSize = 8 << 20
	data := append([]byte("fLaC\x00\x00\x00\x22"), streamInfo...)
	data = append(data, 0x86, byte((pictureBlockSize>>16)&0xFF), byte((pictureBlockSize>>8)&0xFF), byte(pictureBlockSize&0xFF))
	prefix := make([]byte, 8)
	binary.BigEndian.PutUint32(prefix[0:4], 3)
	binary.BigEndian.PutUint32(prefix[4:8], uint32(len("image/jpeg")))
	data = append(data, prefix...)
	data = append(data, []byte("image/jpeg")...)
	logicalSize := int64(4+4+len(streamInfo)+4) + pictureBlockSize
	reader := &sparseReadSeeker{data: data, size: logicalSize}
	_, _, _, _, ok := ParseFLAC(reader, logicalSize)
	if !ok {
		t.Fatal("bounded large-picture FLAC did not parse")
	}
	if reader.maxRead > 34 {
		t.Fatalf("large picture requested %d bytes", reader.maxRead)
	}
	if reader.pos != logicalSize {
		t.Fatalf("reader position = %d, want %d", reader.pos, logicalSize)
	}
}
