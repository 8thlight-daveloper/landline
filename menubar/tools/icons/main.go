package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

type point struct{ x, y float64 }

type drawing struct {
	minX, minY, size float64
	outline          []point
}

var (
	viewBoxForm = regexp.MustCompile(`viewBox="(-?\d+) (-?\d+) (\d+) (\d+)"`)
	pathForm    = regexp.MustCompile(`<path[^>]* d="([^"]+)"`)
	commandForm = regexp.MustCompile(`([MHVZ])(-?[\d.]*)[ ,]?(-?[\d.]*)`)
)

func parse(svg string) (drawing, error) {
	box := viewBoxForm.FindStringSubmatch(svg)
	path := pathForm.FindStringSubmatch(svg)
	if box == nil || path == nil || box[3] != box[4] {
		return drawing{}, errors.New("the drawing needs a square viewBox and one path")
	}
	number := func(text string) float64 { n, _ := strconv.ParseFloat(text, 64); return n }
	d := drawing{minX: number(box[1]), minY: number(box[2]), size: number(box[3])}
	var at point
	for _, command := range commandForm.FindAllStringSubmatch(path[1], -1) {
		switch command[1] {
		case "M":
			at = point{number(command[2]), number(command[3])}
		case "H":
			at.x = number(command[2])
		case "V":
			at.y = number(command[2])
		default:
			continue
		}
		d.outline = append(d.outline, at)
	}
	return d, nil
}

func (d drawing) contains(p point) bool {
	inside := false
	for i, a := range d.outline {
		b := d.outline[(i+1)%len(d.outline)]
		if (a.y > p.y) != (b.y > p.y) && p.x < (b.x-a.x)*(p.y-a.y)/(b.y-a.y)+a.x {
			inside = !inside
		}
	}
	return inside
}

func (d drawing) render(pixels int, ink color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, pixels, pixels))
	scale := d.size / float64(pixels)
	for y := 0; y < pixels; y++ {
		for x := 0; x < pixels; x++ {
			centre := point{d.minX + (float64(x)+0.5)*scale, d.minY + (float64(y)+0.5)*scale}
			if d.contains(centre) {
				img.SetNRGBA(x, y, ink)
			}
		}
	}
	return img
}

var (
	black = color.NRGBA{0, 0, 0, 255}
	dim   = color.NRGBA{0, 0, 0, 102}
	brand = color.NRGBA{0x00, 0x71, 0xCE, 255}
)

type output struct {
	name   string
	pixels int
	ink    color.NRGBA
}

var outputs = []output{
	{"jack-template-22.png", 22, black},
	{"jack-template-44.png", 44, black},
	{"jack-dimmed-22.png", 22, dim},
	{"jack-dimmed-44.png", 44, dim},
	{"jack-about-440.png", 440, brand},
}

var iconsetOutputs = []output{
	{"icon_16x16.png", 16, brand},
	{"icon_16x16@2x.png", 32, brand},
	{"icon_32x32.png", 32, brand},
	{"icon_32x32@2x.png", 64, brand},
	{"icon_128x128.png", 128, brand},
	{"icon_128x128@2x.png", 256, brand},
	{"icon_256x256.png", 256, brand},
	{"icon_256x256@2x.png", 512, brand},
	{"icon_512x512.png", 512, brand},
	{"icon_512x512@2x.png", 1024, brand},
}

const usage = "usage: icons <jack.svg> <output folder>\n       icons -iconset <jack.svg> <folder.iconset>"

func main() {
	args := os.Args[1:]
	outs := outputs
	if len(args) > 0 && args[0] == "-iconset" {
		args, outs = args[1:], iconsetOutputs
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := generate(args[0], args[1], outs); err != nil {
		fmt.Fprintln(os.Stderr, "icons:", err)
		os.Exit(1)
	}
}

func generate(svgPath, folder string, outs []output) error {
	raw, err := os.ReadFile(svgPath)
	if err != nil {
		return err
	}
	d, err := parse(string(raw))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	for _, each := range outs {
		file, err := os.Create(filepath.Join(folder, each.name))
		if err != nil {
			return err
		}
		encodeErr := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(file, d.render(each.pixels, each.ink))
		if closeErr := file.Close(); encodeErr == nil {
			encodeErr = closeErr
		}
		if encodeErr != nil {
			return encodeErr
		}
	}
	return nil
}
