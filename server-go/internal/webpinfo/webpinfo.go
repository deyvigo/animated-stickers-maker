// Package webpinfo reads size and duration straight from a WebP file's
// RIFF container. It replaces the Python server's Pillow dependency: ffprobe
// can't read animated WebP written by libwebp_anim, so we parse the
// container ourselves (VP8X canvas size, ANMF frame durations).
package webpinfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

type Info struct {
	Width    int
	Height   int
	Duration float64 // seconds; 0 for stills and single-frame files
	Frames   int
}

func le24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }

// ReadFile parses the WebP at path.
func ReadFile(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	return Parse(data)
}

// Parse extracts Info from the bytes of a WebP file.
func Parse(data []byte) (Info, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return Info{}, errors.New("not a WebP file")
	}
	// Trust the RIFF size only as an upper bound; truncated files are errors.
	end := 8 + int(binary.LittleEndian.Uint32(data[4:8]))
	if end > len(data) {
		return Info{}, errors.New("truncated WebP file")
	}
	data = data[:end]

	var (
		info       Info
		haveSize   bool
		totalMs    int
		sawContent bool
	)
	for pos := 12; pos+8 <= len(data); {
		fourcc := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			return Info{}, fmt.Errorf("chunk %q overruns file", fourcc)
		}
		payload := data[body : body+size]

		switch fourcc {
		case "VP8X":
			if len(payload) < 10 {
				return Info{}, errors.New("short VP8X chunk")
			}
			info.Width = le24(payload[4:7]) + 1
			info.Height = le24(payload[7:10]) + 1
			haveSize = true
		case "ANMF":
			if len(payload) < 16 {
				return Info{}, errors.New("short ANMF chunk")
			}
			totalMs += le24(payload[12:15])
			info.Frames++
			sawContent = true
		case "VP8 ":
			if !haveSize {
				if len(payload) < 10 || payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
					return Info{}, errors.New("bad VP8 bitstream")
				}
				info.Width = int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff)
				info.Height = int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff)
				haveSize = true
			}
			sawContent = true
		case "VP8L":
			if !haveSize {
				if len(payload) < 5 || payload[0] != 0x2f {
					return Info{}, errors.New("bad VP8L bitstream")
				}
				bits := binary.LittleEndian.Uint32(payload[1:5])
				info.Width = int(bits&0x3fff) + 1
				info.Height = int((bits>>14)&0x3fff) + 1
				haveSize = true
			}
			sawContent = true
		}
		pos = body + size + (size & 1) // chunks are padded to even length
	}

	if !haveSize || !sawContent {
		return Info{}, errors.New("no image data found")
	}
	if info.Frames == 0 {
		info.Frames = 1
	}
	// Mirror the Python server: a single frame has no meaningful duration.
	if info.Frames > 1 {
		info.Duration = float64(totalMs) / 1000.0
	}
	return info, nil
}
