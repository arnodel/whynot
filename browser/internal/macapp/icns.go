package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"

	"golang.org/x/image/draw"
)

// icnsSizes are the icon file's entries: their type, and the width of the
// PNG each holds. Types with "@2x" sizes are the Retina versions.
var icnsSizes = []struct {
	kind  string
	width int
}{
	{"icp4", 16}, {"icp5", 32}, {"icp6", 64},
	{"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024},
	{"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
}

// icns returns img as an icon file (.icns): a header, then an entry per
// size, each a type, a length and a PNG, the lengths counting their own
// headers.
func icns(img image.Image) ([]byte, error) {
	var body bytes.Buffer
	for _, s := range icnsSizes {
		scaled := image.NewNRGBA(image.Rect(0, 0, s.width, s.width))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Src, nil)
		var p bytes.Buffer
		if err := png.Encode(&p, scaled); err != nil {
			return nil, err
		}
		body.WriteString(s.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+p.Len()))
		body.Write(p.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}
