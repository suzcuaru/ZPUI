package tray

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

var (
	cDark    = color.RGBA{0x1a, 0x1a, 0x2e, 0xff}
	cBg      = color.RGBA{0x0f, 0x0f, 0x23, 0xff}
	cRing    = color.RGBA{0x7c, 0x4d, 0xff, 0xff}
	cRing2   = color.RGBA{0x00, 0xe5, 0xff, 0xff}
	cDot1    = color.RGBA{0x00, 0xe5, 0xff, 0xff}
	cDot2    = color.RGBA{0xff, 0x40, 0x81, 0xff}
	cDot3    = color.RGBA{0x7c, 0x4d, 0xff, 0xff}
	cShield  = color.RGBA{0x1a, 0x1a, 0x2e, 0xff}
	cStroke  = color.RGBA{0x7c, 0x4d, 0xff, 0xff}
	cZ       = color.RGBA{0xff, 0xff, 0xff, 0xff}
	cTrail   = color.RGBA{0x00, 0xe5, 0xff, 0xff}
	cTrail2  = color.RGBA{0xff, 0x40, 0x81, 0xff}
	cGlow    = color.RGBA{0x7c, 0x4d, 0xff, 0x26}
	cGlow2   = color.RGBA{0x00, 0xe5, 0xff, 0x33}
)

func DrawShieldIcon(img *image.RGBA) {
	b := img.Bounds()
	sz := b.Dx()
	cx, cy := float64(sz)/2, float64(sz)/2

	// background
	draw.Draw(img, b, image.NewUniform(cBg), image.Point{}, draw.Src)

	// helper
	drawCircle := func(x, y, r float64, c color.Color) {
		rr := int(math.Ceil(r))
		ix, iy := int(x), int(y)
		for dy := -rr; dy <= rr; dy++ {
			for dx := -rr; dx <= rr; dx++ {
				if float64(dx*dx+dy*dy) <= r*r+0.5 {
					img.Set(ix+dx, iy+dy, c)
				}
			}
		}
	}

	fillEllipse := func(x, y, rx, ry, angle float64, c color.Color) {
		sa, ca := math.Sin(angle), math.Cos(angle)
		rr := int(math.Ceil(max(rx, ry)))
		ix, iy := int(x), int(y)
		for dy := -rr; dy <= rr; dy++ {
			for dx := -rr; dx <= rr; dx++ {
				px, py := float64(dx), float64(dy)
				ux := px*ca + py*sa
				vy := -px*sa + py*ca
				el := (ux*ux)/(rx*rx) + (vy*vy)/(ry*ry)
				if el <= 1.0 {
					img.Set(ix+dx, iy+dy, c)
				}
			}
		}
	}

	drawLine := func(x1, y1, x2, y2 float64, c color.Color, w float64) {
		dx := x2 - x1
		dy := y2 - y1
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist == 0 {
			return
		}
		steps := int(dist * 2)
		if steps < 1 {
			steps = 1
		}
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			px := x1 + dx*t
			py := y1 + dy*t
			drawCircle(px, py, w/2, c)
		}
	}

	drawShieldPath := func(x, y, s float64, fill, stroke color.Color, sw float64) {
		n := 7
		points := make([][2]float64, n)
		angles := []float64{-math.Pi / 2, -0.85, -0.3, 0, 0.3, 0.85, math.Pi/2 + 0.3}
		radii := []float64{s, s * 0.95, s * 0.85, s * 0.65, s * 0.85, s * 0.95, s}
		for i := 0; i < n; i++ {
			r := radii[i]
			if i >= 3 && i <= 5 {
				r *= 0.9
			}
			points[i] = [2]float64{x + r*math.Cos(angles[i]), y + r*math.Sin(angles[i])}
		}

		// fill poly
		minY, maxY := y-s, y+s
		minX, maxX := x-s, x+s
		for py := int(minY) - 1; py <= int(maxY)+1; py++ {
			for px := int(minX) - 1; px <= int(maxX)+1; px++ {
				if px < 0 || px >= sz || py < 0 || py >= sz {
					continue
				}
				// point-in-polygon ray casting
				fx, fy := float64(px), float64(py)
				inside := false
				j := n - 1
				for i := 0; i < n; i++ {
					xi, yi := points[i][0], points[i][1]
					xj, yj := points[j][0], points[j][1]
					if (yi > fy) != (yj > fy) && fx < (xj-xi)*(fy-yi)/(yj-yi)+xi {
						inside = !inside
					}
					j = i
				}
				if inside {
					img.Set(px, py, fill)
				}
			}
		}

		// stroke
		if stroke != nil {
			for i := 0; i < n; i++ {
				next := (i + 1) % n
				drawLine(points[i][0], points[i][1], points[next][0], points[next][1], stroke, sw)
			}
		}
	}

	// scale factor
	scale := float64(sz) / 256.0

	// orbital rings
	fillEllipse(cx, cy, 100*scale, 36*scale, -30*math.Pi/180, cRing)
	fillEllipse(cx, cy, 100*scale, 36*scale, 30*math.Pi/180, cRing2)

	// orbital dots
	drawCircle(cx-86*scale*math.Cos(-30*math.Pi/180), cy-86*scale*math.Sin(-30*math.Pi/180), 5*scale, cDot1)
	drawCircle(cx+86*scale*math.Cos(-30*math.Pi/180), cy+86*scale*math.Sin(-30*math.Pi/180), 5*scale, cDot2)
	drawCircle(cx, cy+72*scale, 4*scale, cDot3)

	// shield
	drawShieldPath(cx, cy+10*scale, 52*scale, cShield, cStroke, 3*scale)

	// glow
	drawCircle(cx, cy+10*scale, 28*scale, cGlow)
	drawCircle(cx, cy+10*scale, 12*scale, cGlow2)

	// Z letter
	zw := 3.5 * scale
	zx1, zy1 := cx-16*scale, cy-10*scale
	zx2, zy2 := cx+16*scale, cy-10*scale
	zx3, zy3 := cx-16*scale, cy+10*scale
	zx4, zy4 := cx+16*scale, cy+10*scale
	drawLine(zx1, zy1, zx2, zy2, cZ, zw)
	drawLine(zx2, zy2, zx3, zy3, cZ, zw)
	drawLine(zx3, zy3, zx4, zy4, cZ, zw)

	// energy trails
	drawLine(52*scale, 80*scale, 108*scale, 116*scale, cTrail, 2*scale)
	drawLine(148*scale, 164*scale, 204*scale, 140*scale, cTrail2, 2*scale)
}
