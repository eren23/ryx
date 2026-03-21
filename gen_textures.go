//go:build ignore

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

func makeBanded(path string, c1, c2 color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		c := c1
		if (y/4)%2 == 1 {
			c = c2
		}
		for x := 0; x < 32; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func main() {
	stone1 := color.RGBA{0x78, 0x78, 0x78, 0xFF}
	stone2 := color.RGBA{0x64, 0x64, 0x64, 0xFF}
	if err := makeBanded("examples/assets/wall_stone.png", stone1, stone2); err != nil {
		panic(err)
	}

	moss1 := color.RGBA{0x5A, 0x6E, 0x50, 0xFF}
	moss2 := color.RGBA{0x4A, 0x5E, 0x40, 0xFF}
	if err := makeBanded("examples/assets/wall_moss.png", moss1, moss2); err != nil {
		panic(err)
	}
}
