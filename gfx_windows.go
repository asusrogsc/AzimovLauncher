// Dessin GDI+ : images et polices embarquées, rectangles arrondis, texte.
package main

import (
	"syscall"
	"unsafe"
)

const (
	alignNear   = 0
	alignCenter = 1
	alignFar    = 2

	fontRegular = 0
	fontBold    = 1
	unitPixel   = 2
)

type gfx struct {
	g     uintptr // GpGraphics
	fonts uintptr // GpFontCollection privée
	fam   map[string]uintptr
	cache map[string]uintptr // police (famille|taille|style) -> GpFont
}

var gdipToken uintptr

func gdipStart() bool {
	in := gdiplusStartupInput{Version: 1}
	r, _, _ := pGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&in)), 0)
	return r == 0
}

// Image GDI+ depuis des octets (PNG / JPG embarqués).
func loadImage(data []byte) uintptr {
	if len(data) == 0 {
		return 0
	}
	stream, _, _ := pSHCreateMemStream.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if stream == 0 {
		return 0
	}
	var img uintptr
	pGdipCreateBitmapFromStream.Call(stream, uintptr(unsafe.Pointer(&img)))
	// IStream::Release (3e entrée de la vtable) : GDI+ garde sa propre référence
	type iunknown struct{ vtbl *[3]uintptr }
	obj := (*iunknown)(unsafe.Pointer(stream)) // pointeur COM renvoyé par Windows
	syscall.SyscallN(obj.vtbl[2], stream)
	return img
}

func imageSize(img uintptr) (int, int) {
	var w, h uint32
	pGdipGetImageWidth.Call(img, uintptr(unsafe.Pointer(&w)))
	pGdipGetImageHeight.Call(img, uintptr(unsafe.Pointer(&h)))
	return int(w), int(h)
}

func newFontCollection(files ...[]byte) uintptr {
	var col uintptr
	pGdipNewPrivateFontCollection.Call(uintptr(unsafe.Pointer(&col)))
	for _, f := range files {
		if len(f) > 0 {
			pGdipPrivateAddMemoryFont.Call(col, uintptr(unsafe.Pointer(&f[0])), uintptr(len(f)))
		}
	}
	return col
}

func newGfxFromHDC(hdc uintptr, fonts uintptr, fam map[string]uintptr, cache map[string]uintptr) *gfx {
	var g uintptr
	pGdipCreateFromHDC.Call(hdc, uintptr(unsafe.Pointer(&g)))
	x := &gfx{g: g, fonts: fonts, fam: fam, cache: cache}
	x.quality()
	return x
}

func newGfxFromImage(img uintptr) *gfx {
	var g uintptr
	pGdipGetImageGraphicsContext.Call(img, uintptr(unsafe.Pointer(&g)))
	x := &gfx{g: g}
	x.quality()
	return x
}

func (x *gfx) quality() {
	pGdipSetSmoothingMode.Call(x.g, 4)     // anticrénelage
	pGdipSetTextRenderingHint.Call(x.g, 4) // texte anticrénelé
	pGdipSetInterpolationMode.Call(x.g, 7) // bicubique haute qualité
	pGdipSetPixelOffsetMode.Call(x.g, 4)   // demi-pixel
}

func (x *gfx) close()          { pGdipDeleteGraphics.Call(x.g) }
func (x *gfx) scale(s float32) { pGdipScaleWorldTransform.Call(x.g, f32(s), f32(s), 0) }
func (x *gfx) image(img uintptr, r rectF) {
	pGdipDrawImageRect.Call(x.g, img, f32(r.X), f32(r.Y), f32(r.W), f32(r.H))
}

// Dessin 1:1 (taille explicite : sinon GDI+ applique le rapport de DPI de l'image).
func (x *gfx) imageAt(img uintptr, px, py, w, h int) {
	x.image(img, rectF{float32(px), float32(py), float32(w), float32(h)})
}

func solid(argb uint32) uintptr {
	var b uintptr
	pGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&b)))
	return b
}

func (x *gfx) fillRect(r rectF, argb uint32) {
	b := solid(argb)
	pGdipFillRectangle.Call(x.g, b, f32(r.X), f32(r.Y), f32(r.W), f32(r.H))
	pGdipDeleteBrush.Call(b)
}

// Dégradé vertical (haut -> bas) ou horizontal.
func (x *gfx) gradient(r rectF, c1, c2 uint32, horizontal bool) uintptr {
	p1 := [2]float32{r.X, r.Y}
	p2 := [2]float32{r.X, r.Y + r.H + 1}
	if horizontal {
		p2 = [2]float32{r.X + r.W + 1, r.Y}
	}
	var b uintptr
	pGdipCreateLineBrush.Call(uintptr(unsafe.Pointer(&p1)), uintptr(unsafe.Pointer(&p2)), uintptr(c1), uintptr(c2), 0, uintptr(unsafe.Pointer(&b)))
	return b
}

func roundPath(r rectF, rad float32) uintptr {
	var p uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&p)))
	if rad*2 > r.H {
		rad = r.H / 2
	}
	d := rad * 2
	pGdipAddPathArc.Call(p, f32(r.X), f32(r.Y), f32(d), f32(d), f32(180), f32(90))
	pGdipAddPathArc.Call(p, f32(r.X+r.W-d), f32(r.Y), f32(d), f32(d), f32(270), f32(90))
	pGdipAddPathArc.Call(p, f32(r.X+r.W-d), f32(r.Y+r.H-d), f32(d), f32(d), f32(0), f32(90))
	pGdipAddPathArc.Call(p, f32(r.X), f32(r.Y+r.H-d), f32(d), f32(d), f32(90), f32(90))
	pGdipClosePathFigure.Call(p)
	return p
}

func (x *gfx) fillRound(r rectF, rad float32, argb uint32) {
	p := roundPath(r, rad)
	b := solid(argb)
	pGdipFillPath.Call(x.g, b, p)
	pGdipDeleteBrush.Call(b)
	pGdipDeletePath.Call(p)
}

func (x *gfx) fillRoundBrush(r rectF, rad float32, brush uintptr) {
	p := roundPath(r, rad)
	pGdipFillPath.Call(x.g, brush, p)
	pGdipDeletePath.Call(p)
	pGdipDeleteBrush.Call(brush)
}

func (x *gfx) strokeRound(r rectF, rad float32, argb uint32, width float32) {
	p := roundPath(r, rad)
	var pen uintptr
	pGdipCreatePen1.Call(uintptr(argb), f32(width), unitPixel, uintptr(unsafe.Pointer(&pen)))
	pGdipDrawPath.Call(x.g, pen, p)
	pGdipDeletePen.Call(pen)
	pGdipDeletePath.Call(p)
}

func (x *gfx) circle(cx, cy, rad float32, argb uint32) {
	var p uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&p)))
	pGdipAddPathEllipse.Call(p, f32(cx-rad), f32(cy-rad), f32(rad*2), f32(rad*2))
	b := solid(argb)
	pGdipFillPath.Call(x.g, b, p)
	pGdipDeleteBrush.Call(b)
	pGdipDeletePath.Call(p)
}

// Losange plein avec contour (décor des panneaux).
func (x *gfx) diamond(cx, cy, r float32, fill, stroke uint32) {
	pts := [4][2]float32{{cx, cy - r}, {cx + r, cy}, {cx, cy + r}, {cx - r, cy}}
	var p uintptr
	pGdipCreatePath.Call(0, uintptr(unsafe.Pointer(&p)))
	pGdipAddPathPolygon.Call(p, uintptr(unsafe.Pointer(&pts[0])), 4)
	b := solid(fill)
	pGdipFillPath.Call(x.g, b, p)
	pGdipDeleteBrush.Call(b)
	var pen uintptr
	pGdipCreatePen1.Call(uintptr(stroke), f32(1.2), unitPixel, uintptr(unsafe.Pointer(&pen)))
	pGdipDrawPath.Call(x.g, pen, p)
	pGdipDeletePen.Call(pen)
	pGdipDeletePath.Call(p)
}

func (x *gfx) line(x1, y1, x2, y2 float32, argb uint32, width float32) {
	var pen uintptr
	pGdipCreatePen1.Call(uintptr(argb), f32(width), unitPixel, uintptr(unsafe.Pointer(&pen)))
	pGdipDrawLine.Call(x.g, pen, f32(x1), f32(y1), f32(x2), f32(y2))
	pGdipDeletePen.Call(pen)
}

func (x *gfx) font(family string, size float32, style int) uintptr {
	key := family + "|" + ftoa(size) + "|" + itoa(style)
	if f, ok := x.cache[key]; ok {
		return f
	}
	fam, ok := x.fam[family]
	if !ok {
		pGdipCreateFontFamilyFromName.Call(uintptr(unsafe.Pointer(wstr(family))), x.fonts, uintptr(unsafe.Pointer(&fam)))
		if fam == 0 { // police absente : police système
			pGdipCreateFontFamilyFromName.Call(uintptr(unsafe.Pointer(wstr("Segoe UI"))), 0, uintptr(unsafe.Pointer(&fam)))
		}
		x.fam[family] = fam
	}
	var f uintptr
	pGdipCreateFont.Call(fam, f32(size), uintptr(style), unitPixel, uintptr(unsafe.Pointer(&f)))
	x.cache[key] = f
	return f
}

type textOpt struct {
	align, valign int
	wrap          bool
}

func (x *gfx) text(s string, family string, size float32, style int, r rectF, argb uint32, o textOpt) {
	if s == "" {
		return
	}
	var sf uintptr
	pGdipCreateStringFormat.Call(0, 0, uintptr(unsafe.Pointer(&sf)))
	pGdipSetStringFormatAlign.Call(sf, uintptr(o.align))
	pGdipSetStringFormatLineAlign.Call(sf, uintptr(o.valign))
	pGdipSetStringFormatTrimming.Call(sf, 3) // « … »
	if !o.wrap {
		pGdipSetStringFormatFlags.Call(sf, 0x1000) // pas de retour à la ligne
	}
	b := solid(argb)
	u := syscall.StringToUTF16(s)
	pGdipDrawString.Call(x.g, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), x.font(family, size, style),
		uintptr(unsafe.Pointer(&r)), sf, b)
	pGdipDeleteBrush.Call(b)
	pGdipDeleteStringFormat.Call(sf)
}

// Largeur d'un texte sur une ligne.
func (x *gfx) measure(s string, family string, size float32, style int) float32 {
	if s == "" {
		return 0
	}
	var sf uintptr
	pGdipCreateStringFormat.Call(0, 0, uintptr(unsafe.Pointer(&sf)))
	pGdipSetStringFormatFlags.Call(sf, 0x1000)
	u := syscall.StringToUTF16(s)
	layout := rectF{0, 0, 10000, 1000}
	var out rectF
	pGdipMeasureString.Call(x.g, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), x.font(family, size, style),
		uintptr(unsafe.Pointer(&layout)), sf, uintptr(unsafe.Pointer(&out)), 0, 0)
	pGdipDeleteStringFormat.Call(sf)
	return out.W
}
