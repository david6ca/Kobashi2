package assets

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"
)

func checkConnectionIcon(t *testing.T, data []byte) {
	t.Helper()
	icon, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	bounds := icon.Bounds()
	if bounds.Dx() != bounds.Dy() {
		t.Fatal("connection icon must be square", bounds)
	}
	alpha := func(x, y int) uint32 {
		_, _, _, a := icon.At(bounds.Min.X+x*bounds.Dx()/512, bounds.Min.Y+y*bounds.Dy()/512).RGBA()
		return a
	}
	for _, point := range []image.Point{{128, 256}, {256, 256}, {384, 256}, {128, 208}, {384, 208}} {
		if alpha(point.X, point.Y) != 0xffff {
			t.Fatal("connection icon must have solid circles and a connecting line", bounds, point)
		}
	}
	for _, point := range []image.Point{{0, 0}, {256, 128}, {256, 384}, {256, 192}, {256, 320}} {
		if alpha(point.X, point.Y) != 0 {
			t.Fatal("connection icon must remain transparent outside its circles and line", bounds, point)
		}
	}
}

func TestConnectionIconAssets(t *testing.T) {
	checkConnectionIcon(t, Artwork)
	if len(Icon) < 6 || binary.LittleEndian.Uint16(Icon[2:4]) != 1 {
		t.Fatal("invalid Windows icon header")
	}
	count := int(binary.LittleEndian.Uint16(Icon[4:6]))
	if count != 7 || len(Icon) < 6+count*16 {
		t.Fatal("Windows icon must include all seven native sizes")
	}
	for i, size := range []int{16, 24, 32, 48, 64, 128, 256} {
		entry := Icon[6+i*16 : 6+(i+1)*16]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		length := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if width != size || height != size || length <= 0 || offset < 6+count*16 ||
			offset > len(Icon) || length > len(Icon)-offset {
			t.Fatal("invalid Windows icon frame", size, width, height, offset, length)
		}
		checkConnectionIcon(t, Icon[offset:offset+length])
	}
}
