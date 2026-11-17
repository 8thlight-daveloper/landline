package main

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const svg = `<svg viewBox="-1 -1 11 11"><path fill="#0071CE" d="M3 0H6V1H8V2H9V9H7V7H6V9H5V7H4V9H3V7H2V9H0V2H1V1H3Z"/></svg>`

func TestParseReadsTheJackOutline(t *testing.T) {
	d, err := parse(svg)
	if err != nil || d.size != 11 || d.minX != -1 || len(d.outline) != 24 {
		t.Fatalf("%+v %v", d, err)
	}
	if d.outline[0] != (point{3, 0}) || d.outline[1] != (point{6, 0}) || d.outline[2] != (point{6, 1}) {
		t.Fatalf("%v", d.outline[:3])
	}
}

func TestParseRefusesADrawingWithoutAViewBoxOrAPath(t *testing.T) {
	for _, bad := range []string{"", `<svg viewBox="0 0 4 5"><path d="M0 0H1Z"/></svg>`, `<svg viewBox="0 0 4 4"></svg>`} {
		if _, err := parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestTheGridScalesWithoutBlur(t *testing.T) {
	d, _ := parse(svg)
	for _, pixels := range []int{22, 44, 440} {
		img := d.render(pixels, black)
		for y := 0; y < pixels; y++ {
			for x := 0; x < pixels; x++ {
				if a := img.NRGBAAt(x, y).A; a != 0 && a != 255 {
					t.Fatalf("%d px: a soft pixel at %d,%d", pixels, x, y)
				}
			}
		}
	}
	img := d.render(22, black)
	if img.NRGBAAt(0, 0).A != 0 || img.NRGBAAt(9, 2).A != 255 || img.NRGBAAt(2, 20).A != 0 {
		t.Fatal("the shape is not where the drawing puts it")
	}
}

func TestGenerateIsReproducibleAndMakesTheNamedFiles(t *testing.T) {
	src := filepath.Join(t.TempDir(), "jack.svg")
	os.WriteFile(src, []byte(svg), 0o644)
	first, second := t.TempDir(), t.TempDir()
	for _, folder := range []string{first, second} {
		if err := generate(src, folder, outputs); err != nil {
			t.Fatal(err)
		}
	}
	for _, each := range outputs {
		a, _ := os.ReadFile(filepath.Join(first, each.name))
		b, _ := os.ReadFile(filepath.Join(second, each.name))
		if !bytes.Equal(a, b) || len(a) == 0 {
			t.Errorf("%s differs between runs", each.name)
		}
		img, err := png.Decode(bytes.NewReader(a))
		if err != nil || img.Bounds().Dx() != each.pixels {
			t.Errorf("%s: %v", each.name, err)
		}
	}
}

func TestGenerateReportsAMissingDrawing(t *testing.T) {
	if err := generate(filepath.Join(t.TempDir(), "none.svg"), t.TempDir(), outputs); err == nil {
		t.Fatal("want an error")
	}
}

func TestIconsetHoldsEveryNamedSizeInBrandBlue(t *testing.T) {
	src := filepath.Join(t.TempDir(), "jack.svg")
	os.WriteFile(src, []byte(svg), 0o644)
	folder := filepath.Join(t.TempDir(), "AppIcon.iconset")
	if err := generate(src, folder, iconsetOutputs); err != nil {
		t.Fatal(err)
	}
	for _, each := range iconsetOutputs {
		file, err := os.Open(filepath.Join(folder, each.name))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil || img.Bounds().Dx() != each.pixels {
			t.Fatalf("%s: %v", each.name, err)
		}
	}
}

func TestStateImagesAreReproducibleAndTheSolidAndHollowDifferAtTheCentre(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, folder := range []string{first, second} {
		if err := generateStates(folder); err != nil {
			t.Fatal(err)
		}
	}
	centres := map[string]color.NRGBA{}
	for _, each := range stateOutputs {
		a, _ := os.ReadFile(filepath.Join(first, each.name))
		b, _ := os.ReadFile(filepath.Join(second, each.name))
		if !bytes.Equal(a, b) || len(a) == 0 {
			t.Errorf("%s differs between runs", each.name)
		}
		img, err := png.Decode(bytes.NewReader(a))
		if err != nil || img.Bounds().Dx() != each.pixels {
			t.Fatalf("%s: %v", each.name, err)
		}
		centres[each.name] = color.NRGBAModel.Convert(img.At(each.pixels/2, each.pixels/2)).(color.NRGBA)
	}
	for _, pixels := range []string{"16", "32"} {
		solid, hollow := centres["state-connected-"+pixels+".png"], centres["state-disconnected-"+pixels+".png"]
		if solid != systemGreen || hollow.A != 0 {
			t.Errorf("%s px: centre of solid %+v, of hollow %+v", pixels, solid, hollow)
		}
	}
}
