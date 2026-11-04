package webpinfo

import (
	"encoding/binary"
	"testing"
)

func chunk(fourcc string, payload []byte) []byte {
	b := make([]byte, 8, 8+len(payload)+1)
	copy(b, fourcc)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(payload)))
	b = append(b, payload...)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func riff(chunks ...[]byte) []byte {
	var body []byte
	body = append(body, "WEBP"...)
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := make([]byte, 8)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(body)))
	return append(out, body...)
}

func put24(b []byte, v int) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }

func vp8x(w, h int) []byte {
	p := make([]byte, 10)
	put24(p[4:], w-1)
	put24(p[7:], h-1)
	return chunk("VP8X", p)
}

func anmf(durationMs int) []byte {
	p := make([]byte, 16+4) // header + a stub of frame data
	put24(p[12:], durationMs)
	return chunk("ANMF", p)
}

func TestAnimatedSumsFrameDurations(t *testing.T) {
	data := riff(vp8x(512, 512), anmf(66), anmf(67), anmf(200))
	info, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 512 || info.Height != 512 || info.Frames != 3 {
		t.Fatalf("got %+v", info)
	}
	if info.Duration != 0.333 {
		t.Fatalf("duration = %v, want 0.333", info.Duration)
	}
}

func TestSingleFrameAnimationHasNoDuration(t *testing.T) {
	info, err := Parse(riff(vp8x(512, 512), anmf(500)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Frames != 1 || info.Duration != 0 {
		t.Fatalf("got %+v", info)
	}
}

func TestLossyStill(t *testing.T) {
	p := make([]byte, 10)
	p[3], p[4], p[5] = 0x9d, 0x01, 0x2a
	binary.LittleEndian.PutUint16(p[6:], 512)
	binary.LittleEndian.PutUint16(p[8:], 512)
	info, err := Parse(riff(chunk("VP8 ", p)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 512 || info.Height != 512 || info.Frames != 1 {
		t.Fatalf("got %+v", info)
	}
}

func TestLosslessStill(t *testing.T) {
	p := make([]byte, 5)
	p[0] = 0x2f
	binary.LittleEndian.PutUint32(p[1:], uint32(95)|uint32(47)<<14) // 96x48
	info, err := Parse(riff(chunk("VP8L", p)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 96 || info.Height != 48 {
		t.Fatalf("got %+v", info)
	}
}

func TestStillWithVP8XUsesCanvasSize(t *testing.T) {
	p := make([]byte, 10)
	p[3], p[4], p[5] = 0x9d, 0x01, 0x2a
	binary.LittleEndian.PutUint16(p[6:], 100)
	binary.LittleEndian.PutUint16(p[8:], 100)
	info, err := Parse(riff(vp8x(512, 512), chunk("VP8 ", p)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 512 || info.Height != 512 {
		t.Fatalf("got %+v", info)
	}
}

func TestInvalidInputs(t *testing.T) {
	good := riff(vp8x(512, 512), anmf(100))
	cases := map[string][]byte{
		"empty":         nil,
		"not riff":      []byte("GIF89a......................"),
		"truncated":     good[:len(good)-5],
		"no image data": riff(vp8x(512, 512)),
		"short vp8x":    riff(chunk("VP8X", []byte{1, 2})),
	}
	for name, data := range cases {
		if _, err := Parse(data); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
