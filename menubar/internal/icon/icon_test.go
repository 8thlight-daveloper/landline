package icon

import (
	"bytes"
	stdcolor "image/color"
	"image/png"
	"testing"
)

func decode(t *testing.T, raw []byte) (width int, opaque color) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if c := stdcolor.NRGBAModel.Convert(img.At(x, y)).(stdcolor.NRGBA); c.A > 0 {
				return img.Bounds().Dx(), color{uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)}
			}
		}
	}
	return img.Bounds().Dx(), color{}
}

type color struct{ r, g, b, a uint32 }

func TestTheMenuBarImagesAreBlackOnTransparentAndTheAboutImageIsBrandBlue(t *testing.T) {
	if w, c := decode(t, Template); w != 44 || c != (color{0, 0, 0, 255}) {
		t.Errorf("template %d %+v", w, c)
	}
	if w, c := decode(t, Dimmed); w != 44 || c.r != 0 || c.g != 0 || c.b != 0 || c.a == 0 || c.a >= 255 {
		t.Errorf("dimmed %d %+v", w, c)
	}
	if w, c := decode(t, About); w != 440 || c != (color{0x00, 0x71, 0xCE, 255}) {
		t.Errorf("about %d %+v", w, c)
	}
}
