// Package render turns the embedded wordmark SVG into a grid of
// terminal cells.
//
// Pipeline:
//
//	oksvg rasterize once at high resolution (alpha = glyph coverage)
//	  -> crop to the ink bounding box    (drop the empty SVG canvas)
//	  -> cache the alpha plane           (one pass, reused forever)
//	  -> area-average downsample         (box filter over coverage)
//	  -> exact block-glyph lookup        (sextant/octant atlases)
//
// The wordmark is flat #333333, so only the alpha channel carries
// information; RGB is discarded and the ink color is chosen by the
// caller. Partial coverage is rendered by blending the ink toward the
// assumed terminal background, which keeps edges smooth without ever
// emitting a background color — the output blends into whatever
// terminal it is printed in.
package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/iSundram/Automergent/internal/logo/asset"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// canonicalCanvasW is the rasterization width used for the cached ink
// plane. At the wordmark's ~75% ink fraction this yields roughly 1500
// ink pixels across, an order of magnitude above any realistic output
// width, so the box filter stays well fed at every terminal size.
const canonicalCanvasW = 2048

// inkPlane is the cropped alpha channel of the artwork, cached after
// a single rasterization.
type inkPlane struct {
	w, h  int
	alpha []uint8
}

var (
	inkOnce sync.Once
	ink     inkPlane
	inkErr  error
)

// loadInk rasterizes the embedded asset exactly once per process and
// keeps its cropped alpha plane. Every render afterwards is a cheap
// area-average plus table lookups, so window resizes never re-parse or
// re-draw the SVG.
func loadInk() (inkPlane, error) {
	inkOnce.Do(func() {
		// Fall back to the artwork's nominal aspect if the probe
		// fails, so a measurement problem degrades rather than panics.
		inkErr = fmt.Errorf("artwork unavailable")
		img, _, _, err := rasterize(canonicalCanvasW)
		if err != nil {
			return
		}
		box, ok := alphaBounds(img)
		if !ok {
			inkErr = fmt.Errorf("artwork contains no ink")
			return
		}
		w, h := box.Dx(), box.Dy()
		alpha := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			row := img.Pix[(box.Min.Y+y)*img.Stride+box.Min.X*4:]
			for x := 0; x < w; x++ {
				alpha[y*w+x] = row[x*4+3]
			}
		}
		ink = inkPlane{w: w, h: h, alpha: alpha}
		inkErr = nil
	})
	return ink, inkErr
}

// Aspect reports the wordmark's ink bounding box ratio (width /
// height). It is measured from the embedded asset rather than
// hardcoded, so replacing the SVG automatically re-proportions every
// render.
func Aspect() float64 {
	p, err := loadInk()
	if err != nil || p.h == 0 {
		return 6.0
	}
	return float64(p.w) / float64(p.h)
}

// covEpsilon is the coverage below which a sample counts as empty.
//
// This threshold decides whether enclosed counters AND open
// apertures stay visible. The letterforms' negative spaces sit at
// 0.2-0.5 coverage because the crossbar's antialiased edge bleeds
// into them; painting those samples as ink welds the aperture shut
// (an 'e' turns into an 'o' with a bar). Measured against the
// artwork, 0.16 keeps the aperture 14/14 solid while 0.50 opens it
// to 3/14 with a near-perfect mask (IoU 0.996 vs 0.80); see
// TestThresholdSweep.
const covEpsilon = 0.50

// inkFloor is the minimum blend weight given to a sample that clears
// covEpsilon. Because the renderer emits no background color, a cell
// at 20% coverage would otherwise be 80% background and effectively
// invisible; lifting the floor keeps every drawn cell legible while
// still letting coverage modulate edge weight.
const inkFloor = 0.45

// covLevels quantizes shaped coverage. Fewer distinct values mean
// longer runs of identical color, which both compacts the escape
// sequences and stops edges from shimmering between near-identical
// greys.
const covLevels = 6

// shape maps raw coverage to the blend weight used for the ink,
// applying the floor and quantization described above.
func shape(cov float64) float64 {
	if cov < covEpsilon {
		return 0
	}
	// Rescale [covEpsilon,1] onto [inkFloor,1].
	t := (cov - covEpsilon) / (1 - covEpsilon)
	w := inkFloor + t*(1-inkFloor)
	// Quantize to covLevels steps within [inkFloor,1].
	step := math.Round((w-inkFloor)/(1-inkFloor)*float64(covLevels-1)) / float64(covLevels-1)
	return inkFloor + step*(1-inkFloor)
}

// Cell is one character cell of the rendered wordmark.
type Cell struct {
	Glyph string
	// FG is the foreground color. Only meaningful when Glyph is not
	// blank.
	FG color.RGBA
	// Cov is the mean raw coverage over the cell's inked subpixels,
	// kept for verification and tests. Zero for a blank cell.
	Cov float64
	// Ink is the fraction of the cell's subpixels that are inked.
	Ink float64
}

// Grid is a rendered wordmark.
type Grid struct {
	Cells []Cell
	Cols  int
	Rows  int
}

// At returns the cell at (x, y).
func (g *Grid) At(x, y int) Cell { return g.Cells[y*g.Cols+x] }

// Inker maps a horizontal position (0..1 across the wordmark) and a
// coverage (0..1) to a final foreground color.
type Inker func(u, coverage float64) color.RGBA

// Options controls a render.
type Options struct {
	// Cols is the output width in terminal cells.
	Cols int
	// Rows overrides the automatic height, in terminal cells. Zero
	// derives it from Cols and the wordmark aspect.
	Rows int
	// Ink resolves the foreground color for a given position and
	// coverage. Required.
	Ink Inker
	// Glyphs selects the cell subdivision. The zero value is
	// GlyphQuadrant, which is the best-scoring widely-supported set.
	Glyphs GlyphSet
}

// RowsFor returns the cell height that preserves the wordmark aspect
// at the given width, for the default quadrant glyphs.
func RowsFor(cols int) int { return GlyphQuadrant.rowsFor(cols) }

// rowsFor returns the cell height that preserves the wordmark aspect
// at the given width for this glyph set.
//
// Row count is a count of cells, and a terminal cell is about twice as
// tall as it is wide no matter how it is subdivided, so the answer does
// not depend on the glyph set. Subdivision changes the density of the
// sampling grid inside those cells, not the cell geometry.
func (g GlyphSet) rowsFor(cols int) int {
	rows := int(math.Round(float64(cols) / Aspect() / 2))
	if rows < 1 {
		rows = 1
	}
	return rows
}

// Render rasterizes the wordmark into a cell grid.
func Render(opts Options) (*Grid, error) {
	if opts.Cols <= 0 {
		return nil, fmt.Errorf("cols must be > 0, got %d", opts.Cols)
	}
	if opts.Ink == nil {
		return nil, fmt.Errorf("render: Ink function is required")
	}
	rows := opts.Rows
	if rows == 0 {
		rows = opts.Glyphs.rowsFor(opts.Cols)
	}

	// Subpixel grid: each cell carries subW x subH samples.
	subW, subH := opts.Glyphs.dims()
	sw, sh := opts.Cols*subW, rows*subH

	cov, err := coverage(sw, sh)
	if err != nil {
		return nil, err
	}

	cells := make([]Cell, 0, opts.Cols*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < opts.Cols; x++ {
			// Build the inked pattern for this cell and accumulate the
			// coverage of the subpixels that made the cut. Coverage
			// drives color; the pattern drives shape. Separating the
			// two is what keeps counters open: a hole becomes an
			// uninked subpixel rather than a dimmer shade of ink.
			var pattern uint16
			var sum float64
			var n int
			for sy := 0; sy < subH; sy++ {
				for sx := 0; sx < subW; sx++ {
					c := cov[(y*subH+sy)*sw+(x*subW+sx)]
					if w := shape(c); w > 0 {
						pattern |= 1 << (sy*subW + sx)
						sum += w
						n++
					}
				}
			}

			cell := Cell{Glyph: opts.Glyphs.glyphFor(pattern)}
			if n > 0 {
				cell.Cov = sum / float64(n)
				cell.Ink = float64(n) / float64(subW*subH)
				u := 0.0
				if opts.Cols > 1 {
					u = float64(x) / float64(opts.Cols-1)
				}
				cell.FG = opts.Ink(u, cell.Cov)
			}
			cells = append(cells, cell)
		}
	}
	return &Grid{Cells: cells, Cols: opts.Cols, Rows: rows}, nil
}

// coverage downsamples the cached ink plane to a sw x sh grid of ink
// coverage in [0,1].
func coverage(sw, sh int) ([]float64, error) {
	p, err := loadInk()
	if err != nil {
		return nil, err
	}
	return downsample(p, sw, sh), nil
}

// rasterize draws the embedded artwork onto a transparent canvas of
// the given width, preserving the SVG's own viewBox proportions. The
// alpha channel is exactly ink coverage, since the artwork is fully
// opaque wherever it paints.
func rasterize(canvasW int) (img *image.RGBA, w, h int, err error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(asset.LogoSVG), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("parse embedded svg: %w", err)
	}

	vbW, vbH := icon.ViewBox.W, icon.ViewBox.H
	if vbW <= 0 || vbH <= 0 {
		return nil, 0, 0, fmt.Errorf("svg has no usable viewBox (%vx%v)", vbW, vbH)
	}
	canvasH := int(math.Round(float64(canvasW) * vbH / vbW))
	if canvasH < 1 {
		canvasH = 1
	}
	icon.SetTarget(0, 0, float64(canvasW), float64(canvasH))

	rgba := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	scanner := rasterx.NewScannerGV(canvasW, canvasH, rgba, rgba.Bounds())
	icon.Draw(rasterx.NewDasher(canvasW, canvasH, scanner), 1.0)
	return rgba, canvasW, canvasH, nil
}

// alphaBounds finds the tight bounding box of non-transparent pixels.
func alphaBounds(img *image.RGBA) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+b.Dx()*4]
		for x := 0; x < b.Dx(); x++ {
			if row[x*4+3] == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// downsample area-averages the cached alpha plane into a dw x dh
// coverage grid. Coverage is a linear quantity, so a plain mean is the
// correct filter (no gamma conversion applies).
func downsample(p inkPlane, dw, dh int) []float64 {
	out := make([]float64, dw*dh)
	bw, bh := float64(p.w), float64(p.h)
	for y := 0; y < dh; y++ {
		// Source row span for this output row.
		y0 := int(math.Floor(float64(y) * bh / float64(dh)))
		y1 := int(math.Ceil(float64(y+1) * bh / float64(dh)))
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > p.h {
			y1 = p.h
		}
		for x := 0; x < dw; x++ {
			x0 := int(math.Floor(float64(x) * bw / float64(dw)))
			x1 := int(math.Ceil(float64(x+1) * bw / float64(dw)))
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > p.w {
				x1 = p.w
			}
			var sum uint64
			var n int
			for yy := y0; yy < y1; yy++ {
				base := yy * p.w
				for xx := x0; xx < x1; xx++ {
					sum += uint64(p.alpha[base+xx])
					n++
				}
			}
			if n > 0 {
				out[y*dw+x] = float64(sum) / float64(n) / 255.0
			}
		}
	}
	return out
}
