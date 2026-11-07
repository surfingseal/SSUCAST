package imageanalysis

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
)

const MaxImagePixels = 64_000_000 // 8-bit decoded + one <=8MP working buffer; reject 16-bit decoders

// Assets keeps the original bytes/hash and emits at most slots readable pieces.
// Tall posters use overlapping strips before resizing; no generative edits.
func Assets(raw []byte, im ImageInput, dir string, slots int) ([]ImageInput, error) {
	if len(raw) > 10<<20 || slots < 1 || slots > 3 {
		return nil, fmt.Errorf("asset byte/part cap")
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(raw))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels || cfg.Width > 30000 || cfg.Height > 30000 {
		return nil, fmt.Errorf("image dimensions/decoder rejected (%dx%d)", cfg.Width, cfg.Height)
	}
	if cfg.ColorModel == color.RGBA64Model || cfg.ColorModel == color.NRGBA64Model || cfg.ColorModel == color.Gray16Model {
		return nil, fmt.Errorf("16-bit image memory budget rejected")
	}
	if cfg.Width < 100 && cfg.Height < 100 {
		return nil, fmt.Errorf("decorative image")
	}
	img, _, e := image.Decode(bytes.NewReader(raw))
	if e != nil {
		return nil, fmt.Errorf("image decode failed")
	}
	originalHash := digest(raw)
	originalFile := filepath.Join(dir, originalHash+".original")
	if e = os.WriteFile(originalFile, raw, 0600); e != nil {
		return nil, e
	}
	parts := 1
	if cfg.Height > cfg.Width*3 && slots > 1 {
		parts = int(math.Ceil(float64(cfg.Height) / float64(cfg.Width*3)))
		if parts > slots {
			parts = slots
		}
	}
	result := []ImageInput{}
	for i := 0; i < parts; i++ {
		y0, y1 := i*cfg.Height/parts, (i+1)*cfg.Height/parts
		if parts > 1 {
			if i > 0 {
				y0 -= 96
			}
			if i+1 < parts {
				y1 += 96
			}
		}
		if y0 < 0 {
			y0 = 0
		}
		if y1 > cfg.Height {
			y1 = cfg.Height
		}
		w, h := cfg.Width, y1-y0
		scale := math.Min(1, math.Min(2400/float64(w), math.Sqrt(8_000_000/float64(w*h))))
		nw, nh := int(float64(w)*scale), int(float64(h)*scale)
		if nw < 1 || nh < 1 {
			return nil, fmt.Errorf("invalid scaled size")
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		if scale == 1 {
			draw.Draw(dst, dst.Bounds(), img, image.Pt(0, y0), draw.Src)
		} else {
			// Bilinear interpolation retains thin poster glyphs better than nearest-neighbor.
			for y := 0; y < nh; y++ {
				fy := float64(y)*float64(h)/float64(nh) + float64(y0)
				sy := int(fy)
				dy := fy - float64(sy)
				sy1 := sy + 1
				if sy1 >= y1 {
					sy1 = y1 - 1
				}
				for x := 0; x < nw; x++ {
					fx := float64(x) * float64(w) / float64(nw)
					sx := int(fx)
					dx := fx - float64(sx)
					sx1 := sx + 1
					if sx1 >= w {
						sx1 = w - 1
					}
					v := [4]color.RGBA{}
					for j, pt := range []image.Point{{sx, sy}, {sx1, sy}, {sx, sy1}, {sx1, sy1}} {
						r, g, b, a := img.At(pt.X, pt.Y).RGBA()
						v[j] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
					}
					blend := func(k func(color.RGBA) uint8) uint8 {
						return uint8((1-dy)*((1-dx)*float64(k(v[0]))+dx*float64(k(v[1]))) + dy*((1-dx)*float64(k(v[2]))+dx*float64(k(v[3]))))
					}
					dst.SetRGBA(x, y, color.RGBA{blend(func(c color.RGBA) uint8 { return c.R }), blend(func(c color.RGBA) uint8 { return c.G }), blend(func(c color.RGBA) uint8 { return c.B }), 255})
				}
			}
		}
		var b bytes.Buffer
		if e = jpeg.Encode(&b, dst, &jpeg.Options{Quality: 92}); e != nil {
			return nil, e
		}
		if b.Len() > 10<<20 {
			return nil, fmt.Errorf("compressed piece exceeds cap")
		}
		part := im
		part.OriginalSHA256 = originalHash
		part.OriginalFile = originalFile
		part.SHA256 = digest(b.Bytes())
		part.File = filepath.Join(dir, part.SHA256+".jpg")
		part.Width = nw
		part.Height = nh
		part.MIME = "image/jpeg"
		if parts > 1 {
			part.Crop = &ImageCrop{Y: y0, Width: w, Height: h}
		}
		if e = os.WriteFile(part.File, b.Bytes(), 0600); e != nil {
			return nil, e
		}
		result = append(result, part)
	}
	return result, nil
}
