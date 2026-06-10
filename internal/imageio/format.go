package imageio

import (
	"bytes"
	"strings"
	"unicode"
)

type Format string

const (
	FormatUnknown Format = ""
	FormatPNG     Format = "png"
	FormatJPEG    Format = "jpeg"
	FormatGIF     Format = "gif"
	FormatWebP    Format = "webp"
	FormatBMP     Format = "bmp"
	FormatTIFF    Format = "tiff"
	FormatAVIF    Format = "avif"
	FormatSVG     Format = "svg"
	FormatICO     Format = "ico"
)

func detectFormat(head []byte) Format {
	switch {
	case len(head) >= 8 && bytes.Equal(head[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return FormatPNG
	case len(head) >= 3 && bytes.Equal(head[:3], []byte{0xff, 0xd8, 0xff}):
		return FormatJPEG
	case len(head) >= 6 && (bytes.Equal(head[:6], []byte("GIF87a")) || bytes.Equal(head[:6], []byte("GIF89a"))):
		return FormatGIF
	case len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return FormatWebP
	case len(head) >= 2 && bytes.Equal(head[:2], []byte("BM")):
		return FormatBMP
	case len(head) >= 4 && (bytes.Equal(head[:4], []byte{'I', 'I', 42, 0}) || bytes.Equal(head[:4], []byte{'M', 'M', 0, 42})):
		return FormatTIFF
	case isAVIF(head):
		return FormatAVIF
	case len(head) >= 4 && bytes.Equal(head[:4], []byte{0, 0, 1, 0}):
		return FormatICO
	case isSVG(head):
		return FormatSVG
	default:
		return FormatUnknown
	}
}

func isAVIF(head []byte) bool {
	if len(head) < 12 || !bytes.Equal(head[4:8], []byte("ftyp")) {
		return false
	}
	brands := head[8:]
	return bytes.Contains(brands, []byte("avif")) || bytes.Contains(brands, []byte("avis"))
}

func isSVG(head []byte) bool {
	s := strings.TrimLeftFunc(string(head), unicode.IsSpace)
	if strings.HasPrefix(s, "<?xml") {
		end := strings.Index(s, "?>")
		if end == -1 {
			return false
		}
		s = strings.TrimLeftFunc(s[end+2:], unicode.IsSpace)
	}
	return strings.HasPrefix(s, "<svg")
}
