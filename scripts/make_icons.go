//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

type iconImage struct {
	size int
	data []byte
}

func main() {
	src := loadSource("assets/app-icon.png")
	images := make([]iconImage, 0, 7)
	for _, size := range []int{16, 32, 48, 64, 128, 256, 512, 1024} {
		data := renderPNG(src, size)
		writeFile(filepath.Join("assets", "icons", "icon-"+itoa(size)+".png"), data)
		if size != 48 {
			images = append(images, iconImage{size: size, data: data})
		}
	}
	writeICO("assets/app-icon.ico", []iconImage{
		imageBySize(images, 16),
		imageBySize(images, 32),
		{size: 48, data: renderPNG(src, 48)},
		{size: 64, data: renderPNG(src, 64)},
		imageBySize(images, 128),
		imageBySize(images, 256),
	})
	writeICNS("assets/app-icon.icns", images)
}

func loadSource(path string) image.Image {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		log.Fatal(err)
	}
	return transparentBlackCorners(squareCrop(img))
}

func squareCrop(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	size := width
	if height < size {
		size = height
	}
	x0 := bounds.Min.X + (width-size)/2
	y0 := bounds.Min.Y + (height-size)/2
	return img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(x0, y0, x0+size, y0+size))
}

func transparentBlackCorners(img image.Image) image.Image {
	bounds := img.Bounds()
	out := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			r := uint8(r16 >> 8)
			g := uint8(g16 >> 8)
			b := uint8(b16 >> 8)
			a := uint8(a16 >> 8)
			if r < 18 && g < 18 && b < 18 {
				a = 0
			}
			out.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	return out
}

func renderPNG(src image.Image, size int) []byte {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func writeICO(path string, images []iconImage) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(images)))
	offset := 6 + 16*len(images)
	for _, img := range images {
		width := byte(img.size)
		height := byte(img.size)
		if img.size >= 256 {
			width = 0
			height = 0
		}
		buf.WriteByte(width)
		buf.WriteByte(height)
		buf.WriteByte(0)
		buf.WriteByte(0)
		binary.Write(&buf, binary.LittleEndian, uint16(1))
		binary.Write(&buf, binary.LittleEndian, uint16(32))
		binary.Write(&buf, binary.LittleEndian, uint32(len(img.data)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(img.data)
	}
	for _, img := range images {
		buf.Write(img.data)
	}
	writeFile(path, buf.Bytes())
}

func writeICNS(path string, images []iconImage) {
	chunks := map[int]string{
		16:   "icp4",
		32:   "icp5",
		64:   "icp6",
		128:  "ic07",
		256:  "ic08",
		512:  "ic09",
		1024: "ic10",
	}
	var body bytes.Buffer
	for _, img := range images {
		chunkType, ok := chunks[img.size]
		if !ok {
			continue
		}
		body.WriteString(chunkType)
		binary.Write(&body, binary.BigEndian, uint32(len(img.data)+8))
		body.Write(img.data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	writeFile(path, out.Bytes())
}

func writeFile(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Fatal(err)
	}
}

func imageBySize(images []iconImage, size int) iconImage {
	for _, img := range images {
		if img.size == size {
			return img
		}
	}
	log.Fatalf("missing icon size %d", size)
	return iconImage{}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
