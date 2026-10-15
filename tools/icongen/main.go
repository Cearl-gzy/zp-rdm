// icongen 程序化生成应用图标：build/appicon.png (256px) + build/windows/icon.ico
// 图形：深色圆角方块底 + Redis 风格三层堆叠（红色渐变），2x 超采样抗锯齿
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

const (
	size  = 256
	scale = 2
)

type rgb struct{ r, g, b float64 }

var (
	bgColor = rgb{0x16, 0x1a, 0x23}
	// 三层（顶面色），侧面按比例压暗
	topFaces = []rgb{
		{0xef, 0x5b, 0x43},
		{0xd6, 0x49, 0x31},
		{0xbd, 0x3c, 0x27},
	}
)

func roundedRectSDF(x, y, w, h, r float64) float64 {
	// 到圆角矩形边界的有符号距离（负=内部）
	cx, cy := w/2, h/2
	qx := abs(x-cx) - (w/2 - r)
	qy := abs(y-cy) - (h/2 - r)
	ax, ay := max(qx, 0), max(qy, 0)
	return min(max(qx, qy), 0) + sqrt(ax*ax+ay*ay) - r
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func sqrt(v float64) float64 {
	// Newton
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 32; i++ {
		x = (x + v/x) / 2
	}
	return x
}

// pointInPolygon 射线法
func pointInPolygon(x, y float64, poly [][2]float64) bool {
	inside := false
	j := len(poly) - 1
	for i := range poly {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

func shade(c rgb, k float64) rgb { return rgb{c.r * k, c.g * k, c.b * k} }

// slab 三层堆叠的一层（顶面 + 左右侧面）
func slab(cx, topY, w, h, sideH float64) (top, left, right [][2]float64) {
	top = [][2]float64{{cx, topY}, {cx + w/2, topY + h/2}, {cx, topY + h}, {cx - w/2, topY + h/2}}
	left = [][2]float64{{cx - w/2, topY + h/2}, {cx, topY + h}, {cx, topY + h + sideH}, {cx - w/2, topY + h/2 + sideH}}
	right = [][2]float64{{cx, topY + h}, {cx + w/2, topY + h/2}, {cx + w/2, topY + h/2 + sideH}, {cx, topY + h + sideH}}
	return
}

type polyFill struct {
	poly [][2]float64
	col  rgb
}

func render(ss int) []rgb {
	n := size * ss
	buf := make([]rgb, n*n)

	var fills []polyFill
	s := float64(ss)
	cx := 128 * s
	layerW := 148 * s
	layerH := 64 * s
	sideH := 24 * s
	topYs := []float64{46 * s, 92 * s, 138 * s}
	for i, topY := range topYs {
		top, left, right := slab(cx, topY, layerW, layerH, sideH)
		fills = append(fills,
			polyFill{right, shade(topFaces[i], 0.62)},
			polyFill{left, shade(topFaces[i], 0.8)},
			polyFill{top, topFaces[i]},
		)
	}

	rr := 56 * s
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			col := bgColor
			d := roundedRectSDF(px, py, float64(n), float64(n), rr)
			if d > 0 {
				col = rgb{0, 0, 0} // 圆角外透明（后续 alpha 处理）
			}
			for _, f := range fills {
				if pointInPolygon(px, py, f.poly) {
					col = f.col
				}
			}
			buf[y*n+x] = col
		}
	}
	return buf
}

func main() {
	ss := scale
	n := size * ss
	buf := render(ss)

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					c := buf[(y*ss+dy)*n+(x*ss+dx)]
					// 圆角外像素在 render 中标记为黑色 → 视为透明背景
					// 通过 SDF 重新判定 alpha 更准确，这里直接: 背景色视为不透明
					r += c.r
					g += c.g
					b += c.b
				}
			}
			k := float64(ss * ss)
			_ = a
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(min(255, r/k)),
				G: uint8(min(255, g/k)),
				B: uint8(min(255, b/k)),
				A: 255,
			})
		}
	}
	// 圆角外区域置透明（按 SDF 中心判定）
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if roundedRectSDF(float64(x)+0.5, float64(y)+0.5, size, size, 56) > 0.5 {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
			}
		}
	}

	// 1) build/appicon.png
	if err := os.MkdirAll("build/windows", 0o755); err != nil {
		panic(err)
	}
	pngPath := filepath.Join("build", "appicon.png")
	f, err := os.Create(pngPath)
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	_ = f.Close()

	// 2) build/windows/icon.ico（PNG-in-ICO，Vista+ 支持）
	raw, err := os.ReadFile(pngPath)
	if err != nil {
		panic(err)
	}
	ico := make([]byte, 0, 22+len(raw))
	u16 := func(v uint16) { ico = append(ico, byte(v), byte(v>>8)) }
	u32 := func(v uint32) { ico = append(ico, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }
	u16(0)                        // reserved
	u16(1)                        // type: icon
	u16(1)                        // count
	ico = append(ico, 0, 0, 0, 0) // width=256, height=256, colors, reserved
	u16(1)                        // planes
	u16(32)                       // bit count
	u32(uint32(len(raw)))         // bytes in resource
	u32(22)                       // offset
	ico = append(ico, raw...)
	icoPath := filepath.Join("build", "windows", "icon.ico")
	if err := os.WriteFile(icoPath, ico, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("generated %s (%d bytes) and %s (%d bytes)\n", pngPath, len(raw), icoPath, len(ico))
}
