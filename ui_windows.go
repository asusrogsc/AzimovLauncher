// Fenêtre du launcher : dessin, zones cliquables, souris.
package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// Taille de référence (à 100 %) ; tout est mis à l'échelle selon le DPI.
const (
	baseW = 1180
	baseH = 690
)

const (
	colGold      = 0xFFE2C27A
	colGoldDark  = 0xFF9C7A3C
	colInk       = 0xFF2A2118
	colInkSoft   = 0xFF5B4E40
	colPanel     = 0xEFF5F2EC
	colBar       = 0xD8121018
	colWhite     = 0xFFFFFFFF
	colWhiteSoft = 0xCCFFFFFF
	colGreen     = 0xFF3DBE6A
	colRed       = 0xFFD9534F
	colGrey      = 0xFF9A9A9A
)

const (
	fTitle  = "Cinzel AZ Bold"
	fCinzel = "Cinzel"
	fBody   = "Marcellus"
)

type hotID int

const (
	hotNone hotID = iota
	hotSite
	hotVote
	hotRegister
	hotMin
	hotClose
	hotMain
	hotChange
	hotVerify
	hotCache
)

type hot struct {
	id hotID
	r  rectF
}

type UI struct {
	hwnd     uintptr
	scale    float32
	eng      *Engine
	bgImg    uintptr
	logoImg  uintptr
	bgCache  uintptr // fond déjà mis à l'échelle (taille fenêtre)
	bgW, bgH int
	fonts    uintptr
	fam      map[string]uintptr
	fcache   map[string]uintptr
	hots     []hot
	hover    hotID
	pressed  hotID
	tracking bool
}

var ui *UI

func (u *UI) toBase(lp uintptr) (float32, float32) {
	return float32(loword(lp)) / u.scale, float32(hiword(lp)) / u.scale
}

func (u *UI) hitAt(x, y float32) hotID {
	for i := len(u.hots) - 1; i >= 0; i-- {
		h := u.hots[i]
		if x >= h.r.X && x < h.r.X+h.r.W && y >= h.r.Y && y < h.r.Y+h.r.H {
			return h.id
		}
	}
	return hotNone
}

func (u *UI) addHot(id hotID, r rectF) { u.hots = append(u.hots, hot{id, r}) }

func (u *UI) invalidate() { pInvalidateRect.Call(u.hwnd, 0, 0) }

// Fond mis à l'échelle une seule fois (recadré pour couvrir la fenêtre).
func (u *UI) ensureBg(w, h int) {
	if u.bgCache != 0 && u.bgW == w && u.bgH == h {
		return
	}
	if u.bgCache != 0 {
		pGdipDisposeImage.Call(u.bgCache)
	}
	var bmp uintptr
	pGdipCreateBitmapFromScan0.Call(uintptr(w), uintptr(h), 0, 0x26200A, 0, uintptr(unsafe.Pointer(&bmp))) // 32bpp ARGB
	g := newGfxFromImage(bmp)
	iw, ih := imageSize(u.bgImg)
	if iw > 0 && ih > 0 {
		sc := float32(w) / float32(iw)
		if s2 := float32(h) / float32(ih); s2 > sc {
			sc = s2
		}
		dw, dh := float32(iw)*sc, float32(ih)*sc
		g.image(u.bgImg, rectF{(float32(w) - dw) / 2, (float32(h) - dh) / 2, dw, dh})
	}
	// léger voile sombre en bas pour la lisibilité
	b := g.gradient(rectF{0, float32(h) * 0.55, float32(w), float32(h) * 0.45}, 0x00000000, 0xB0000000, false)
	pGdipFillRectangle.Call(g.g, b, f32(0), f32(float32(h)*0.55), f32(float32(w)), f32(float32(h)*0.45+1))
	pGdipDeleteBrush.Call(b)
	g.close()
	u.bgCache, u.bgW, u.bgH = bmp, w, h
}

func (u *UI) paint() {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc rect
	pGetClientRect.Call(u.hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if w <= 0 || h <= 0 {
		return
	}
	mem, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mem, bmp)

	u.ensureBg(w, h)
	g := newGfxFromHDC(mem, u.fonts, u.fam, u.fcache)
	g.imageAt(u.bgCache, 0, 0, w, h)
	g.scale(u.scale)
	u.hots = u.hots[:0]
	u.draw(g)
	g.close()

	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, SRCCOPY)
	pSelectObject.Call(mem, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
}

func (u *UI) draw(g *gfx) {
	s := u.eng.Snapshot()

	// ---- logo
	if u.logoImg != 0 {
		g.image(u.logoImg, rectF{34, 6, 360, 180})
	}

	// ---- liens du haut (de droite à gauche) + fenêtre
	// réduire / fermer (dessinés, pas de glyphes : rien ne dépend des polices)
	mc := u.hoverCol(hotMin, colWhiteSoft, colWhite)
	g.line(baseW-84, 26, baseW-66, 26, mc, 2)
	u.addHot(hotMin, rectF{baseW - 92, 10, 34, 32})
	cc := u.hoverCol(hotClose, colWhiteSoft, colRed)
	g.line(baseW-44, 17, baseW-28, 33, cc, 2)
	g.line(baseW-44, 33, baseW-28, 17, cc, 2)
	u.addHot(hotClose, rectF{baseW - 52, 10, 34, 32})

	x := float32(baseW - 124)
	for _, l := range []struct {
		id    hotID
		label string
	}{{hotRegister, "CRÉER UN COMPTE"}, {hotVote, "VOTER"}, {hotSite, "SITE WEB"}} {
		tw := g.measure(l.label, fTitle, 14, fontRegular)
		x -= tw
		r := rectF{x - 8, 12, tw + 16, 28}
		col := u.hoverCol(l.id, colWhite, colGold)
		if l.id == hotVote { // le vote ressort en doré
			col = u.hoverCol(l.id, colGold, colWhite)
			pill := rectF{r.X - 6, r.Y, r.W + 12, r.H}
			g.fillRound(pill, 14, 0x70000000)
			g.strokeRound(pill, 14, 0xCCE2C27A, 1.2)
		}
		g.text(l.label, fTitle, 14, fontRegular, r, col, textOpt{align: alignCenter, valign: alignCenter})
		u.addHot(l.id, r)
		x -= 30
	}
	ver := "Launcher " + AppVersion
	if s.Manifest != nil && s.Manifest.Version != "" {
		ver += " · Patch " + s.Manifest.Version
	}
	g.text(ver, fBody, 12.5, fontRegular, rectF{baseW - 420, 44, 400, 20}, 0xCCFFFFFF, textOpt{align: alignFar})

	// ---- panneau statut
	u.panel(g, rectF{36, 210, 370, 280})
	g.text("STATUT DU ROYAUME", fTitle, 19, fontRegular, rectF{64, 232, 330, 30}, colInk, textOpt{valign: alignCenter})
	g.line(64, 270, 378, 270, 0x40000000, 1)
	srv := s.Server
	realm, auth, world, online, rl := "AZIMOV", false, false, -1, ""
	if srv != nil {
		if srv.Realm != "" {
			realm = strings.ToUpper(srv.Realm)
		}
		auth, world, rl = srv.Auth, srv.World, srv.Realmlist
		if srv.Online != nil {
			online = *srv.Online
		}
	}
	if rl == "" && s.Manifest != nil {
		rl = s.Manifest.Realmlist
	}
	g.text(realm, fTitle, 17, fontRegular, rectF{64, 284, 330, 26}, colInk, textOpt{valign: alignCenter})
	u.stateLine(g, 324, auth, "Connexion en ligne", "Connexion hors ligne", srv == nil)
	u.stateLine(g, 360, world, "Monde en ligne", "Monde hors ligne", srv == nil)
	if online >= 0 {
		g.text(itoa(online), fTitle, 30, fontRegular, rectF{62, 392, 60, 40}, colGoldDark, textOpt{valign: alignCenter})
		lbl := "joueurs connectés"
		if online <= 1 {
			lbl = "joueur connecté"
		}
		ow := g.measure(itoa(online), fTitle, 30, fontRegular)
		g.text(lbl, fBody, 16, fontRegular, rectF{64 + ow + 8, 396, 260, 34}, colInk, textOpt{valign: alignCenter})
	}
	if rl != "" {
		g.text("set realmlist "+rl, fBody, 13, fontRegular, rectF{64, 446, 330, 22}, colInkSoft, textOpt{valign: alignCenter})
	}

	// ---- panneau actualités
	u.panel(g, rectF{426, 210, 718, 280})
	g.text("ACTUALITÉS", fTitle, 19, fontRegular, rectF{454, 232, 660, 30}, colInk, textOpt{valign: alignCenter})
	g.line(454, 270, 1116, 270, 0x40000000, 1)
	if srv != nil && len(srv.News) > 0 {
		y := float32(282)
		for i, n := range srv.News {
			if i >= 2 {
				break
			}
			g.text(n.Date, fBody, 13, fontRegular, rectF{454, y, 660, 18}, colInkSoft, textOpt{})
			g.text(strings.ToUpper(n.Title), fTitle, 15, fontRegular, rectF{454, y + 18, 660, 24}, colInk, textOpt{valign: alignCenter})
			g.text(n.Text, fBody, 14.5, fontRegular, rectF{454, y + 44, 660, 42}, colInk, textOpt{wrap: true})
			if i == 0 {
				g.line(454, y+96, 1116, y+96, 0x26000000, 1)
			}
			y += 104
		}
	} else {
		g.text("Aucune actualité pour le moment.", fBody, 15, fontRegular, rectF{454, 290, 660, 30}, colInkSoft, textOpt{})
	}

	// ---- barre du bas
	bar := rectF{24, 512, baseW - 48, 158}
	g.fillRound(bar, 14, colBar)
	g.strokeRound(bar, 14, 0x80E2C27A, 1.2)

	status := s.Status
	if status == "" {
		status = "Chargement…"
	}
	g.text(status, fBody, 18, fontRegular, rectF{52, 528, 790, 30}, colWhite, textOpt{valign: alignCenter})

	// progression
	pb := rectF{52, 568, 790, 18}
	g.fillRound(pb, 9, 0x50FFFFFF)
	g.strokeRound(pb, 9, 0x90E2C27A, 1)
	if s.Progress > 0 {
		pw := pb.W * float32(minf(s.Progress, 1))
		if pw > 4 {
			fill := rectF{pb.X + 2, pb.Y + 2, pw - 4, pb.H - 4}
			g.fillRoundBrush(fill, 7, g.gradient(fill, 0xFFB8862E, 0xFFF4E2A8, true))
		}
	} else if s.Busy && s.Progress < 0 {
		g.fillRound(rectF{pb.X + 2, pb.Y + 2, pb.W * 0.25, pb.H - 4}, 7, 0x90E2C27A)
	}
	g.text(s.Detail, fBody, 13.5, fontRegular, rectF{52, 590, 790, 22}, 0xDDFFFFFF, textOpt{valign: alignCenter})

	// dossier
	g.text("Dossier du jeu :", fBody, 14, fontRegular, rectF{52, 626, 120, 26}, 0xCCFFFFFF, textOpt{valign: alignCenter})
	g.text(s.Settings.GameDir, fBody, 14, fontRegular, rectF{170, 626, 440, 26}, colWhite, textOpt{valign: alignCenter})
	for _, l := range []struct {
		id    hotID
		label string
		x     float32
	}{{hotChange, "Changer", 622}, {hotVerify, "Vérifier les fichiers", 712}} {
		tw := g.measure(l.label, fBody, 14.5, fontRegular)
		r := rectF{l.x, 626, tw + 4, 26}
		col := uint32(colGold)
		if s.Busy {
			col = 0x80E2C27A
		} else if u.hover == l.id {
			col = colWhite
		}
		g.text(l.label, fBody, 14.5, fontRegular, r, col, textOpt{valign: alignCenter})
		if !s.Busy {
			u.addHot(l.id, r)
		}
	}

	// bouton principal
	label := "JOUER"
	enabled := true
	switch s.Phase {
	case PhaseConnecting, PhaseChecking, PhaseSelfUpdate, PhaseLaunching:
		label, enabled = "PATIENTE…", false
	case PhaseNeedInstall:
		label = "INSTALLER"
	case PhaseNeedUpdate:
		label = "METTRE À JOUR"
	case PhaseDownloading:
		label = "PAUSE"
	case PhaseError:
		label = "RÉESSAYER"
	}
	btn := rectF{872, 530, 256, 76}
	top, bot := uint32(0xFFFBF8F1), uint32(0xFFD9D2C3)
	if !enabled {
		top, bot = 0xFFB9B4AA, 0xFF8F8A80
	} else if u.pressed == hotMain {
		top, bot = 0xFFD9D2C3, 0xFFFBF8F1
	} else if u.hover == hotMain {
		top, bot = 0xFFFFFFFF, 0xFFEADFC4
	}
	g.fillRoundBrush(btn, 8, g.gradient(btn, top, bot, false))
	g.strokeRound(btn, 8, u.hoverCol(hotMain, 0xFFB79A5E, colGold), 2)
	size := float32(28)
	if len([]rune(label)) > 9 {
		size = 22
	}
	g.text(label, fTitle, size, fontRegular, btn, colInk, textOpt{align: alignCenter, valign: alignCenter})
	if enabled {
		u.addHot(hotMain, btn)
	}

	// case « vider le cache »
	cb := rectF{876, 628, 20, 20}
	g.fillRound(cb, 4, 0x30FFFFFF)
	g.strokeRound(cb, 4, colGold, 1.4)
	if s.Settings.ClearCache {
		g.line(cb.X+4, cb.Y+10, cb.X+8.5, cb.Y+15, colGold, 2.2)
		g.line(cb.X+8.5, cb.Y+15, cb.X+16, cb.Y+5, colGold, 2.2)
	}
	g.text("Vider le cache au lancement", fBody, 14.5, fontRegular, rectF{904, 624, 230, 28}, u.hoverCol(hotCache, 0xDDFFFFFF, colWhite), textOpt{valign: alignCenter})
	u.addHot(hotCache, rectF{872, 624, 262, 28})
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (u *UI) hoverCol(id hotID, normal, over uint32) uint32 {
	if u.hover == id {
		return over
	}
	return normal
}

func (u *UI) panel(g *gfx, r rectF) {
	g.fillRound(rectF{r.X + 3, r.Y + 5, r.W, r.H}, 12, 0x50000000) // ombre
	g.fillRound(r, 12, colPanel)
	g.strokeRound(r, 12, 0xB0FFFFFF, 1.5)
	// petits losanges sur les bords, comme l'ancien launcher
	for _, p := range [][2]float32{{r.X, r.Y + r.H/2}, {r.X + r.W, r.Y + r.H/2}} {
		g.diamond(p[0], p[1], 7, 0xFFF5F2EC, 0xFFB79A5E)
	}
}

func (u *UI) stateLine(g *gfx, y float32, ok bool, on, off string, unknown bool) {
	col, txt := uint32(colRed), off
	if ok {
		col, txt = colGreen, on
	}
	if unknown {
		col, txt = colGrey, "…"
	}
	g.circle(80, y+13, 9, 0x40000000)
	g.circle(80, y+13, 7, col)
	g.text(txt, fBody, 16.5, fontRegular, rectF{100, y, 290, 26}, colInk, textOpt{valign: alignCenter})
}

func (u *UI) click(id hotID) {
	s := u.eng.Snapshot()
	switch id {
	case hotClose:
		if s.Phase == PhaseDownloading {
			if msgBox(u.hwnd, "Un téléchargement est en cours. Il reprendra au prochain lancement.\nQuitter quand même ?", "Azimov Online", MB_YESNO|MB_ICONWARNING) != IDYES {
				return
			}
			u.eng.Cancel()
		}
		pPostMessageW.Call(u.hwnd, WM_CLOSE, 0, 0)
	case hotMin:
		pShowWindow.Call(u.hwnd, SW_MINIMIZE)
	case hotSite:
		openURL(u.eng.SiteURL())
	case hotVote:
		openURL(u.eng.VoteURL())
	case hotRegister:
		openURL(u.eng.RegisterURL())
	case hotChange:
		if dir := browseFolder(u.hwnd, "Choisis le dossier d'installation d'Azimov Online (ou ton client Cataclysm 4.3.4 existant) :"); dir != "" {
			u.eng.SetGameDir(dir)
		}
	case hotVerify:
		u.eng.Verify()
	case hotCache:
		u.eng.SetClearCache(!s.Settings.ClearCache)
	case hotMain:
		switch s.Phase {
		case PhaseReady:
			if err := u.eng.Launch(); err != nil {
				msgBox(u.hwnd, fmt.Sprintf("Impossible de lancer le jeu :\n%v\n\nEssaie « Vérifier les fichiers ».", err), "Azimov Online", MB_ICONERROR)
				return
			}
			pShowWindow.Call(u.hwnd, SW_MINIMIZE)
		case PhaseNeedInstall, PhaseNeedUpdate:
			go u.eng.Download()
		case PhaseDownloading:
			u.eng.Cancel()
		case PhaseError:
			go u.eng.refresh(false)
		}
	}
	u.invalidate()
}

// ---------------------------------------------------------------- procédure de fenêtre

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	u := ui
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		if u != nil {
			u.paint()
			return 0
		}
	case WM_NCHITTEST:
		if u != nil {
			pt := point{loword(lp), hiword(lp)}
			pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
			x, y := float32(pt.X)/u.scale, float32(pt.Y)/u.scale
			if u.hitAt(x, y) == hotNone && y < 200 {
				return HTCAPTION // on déplace la fenêtre par le haut
			}
			return HTCLIENT
		}
	case WM_SETCURSOR:
		if u != nil && u.hover != hotNone && loword(lp) == HTCLIENT {
			c, _, _ := pLoadCursorW.Call(0, IDC_HAND)
			pSetCursor.Call(c)
			return 1
		}
	case WM_MOUSEMOVE:
		if u != nil {
			if !u.tracking {
				tme := trackMouseEvent{Flags: TME_LEAVE, Hwnd: hwnd}
				tme.Size = uint32(unsafe.Sizeof(tme))
				pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				u.tracking = true
			}
			x, y := u.toBase(lp)
			if h := u.hitAt(x, y); h != u.hover {
				u.hover = h
				u.invalidate()
			}
		}
		return 0
	case WM_MOUSELEAVE:
		if u != nil {
			u.tracking = false
			u.hover, u.pressed = hotNone, hotNone
			u.invalidate()
		}
		return 0
	case WM_LBUTTONDOWN:
		if u != nil {
			x, y := u.toBase(lp)
			u.pressed = u.hitAt(x, y)
			u.invalidate()
		}
		return 0
	case WM_LBUTTONUP:
		if u != nil {
			x, y := u.toBase(lp)
			h := u.hitAt(x, y)
			p := u.pressed
			u.pressed = hotNone
			if h != hotNone && h == p {
				u.click(h)
			}
			u.invalidate()
		}
		return 0
	case WM_TIMER, WM_APP:
		if u != nil {
			u.invalidate()
		}
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func createWindow(u *UI) bool {
	inst, _, _ := pGetModuleHandleW.Call(0)
	icon, _, _ := pLoadIconW.Call(inst, 1) // groupe d'icônes n° 1 (rsrc)
	cursor, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	cls := wstr("AzimovLauncherWnd")
	wc := wndClassEx{Style: CS_HREDRAW | CS_VREDRAW, WndProc: syscall.NewCallback(wndProc), Instance: inst,
		Icon: icon, Cursor: cursor, ClassName: cls, IconSm: icon}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return false
	}
	w, h := int(float32(baseW)*u.scale), int(float32(baseH)*u.scale)
	sw, _, _ := pGetSystemMetrics.Call(SM_CXSCREEN)
	sh, _, _ := pGetSystemMetrics.Call(SM_CYSCREEN)
	x, y := (int(sw)-w)/2, (int(sh)-h)/2
	hwnd, _, _ := pCreateWindowExW.Call(WS_EX_APPWINDOW, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(wstr("Azimov Online"))),
		WS_POPUP|WS_MINIMIZEBOX|WS_SYSMENU, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, inst, 0)
	if hwnd == 0 {
		return false
	}
	u.hwnd = hwnd
	// coins arrondis
	rad := int(16 * u.scale)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(rad), uintptr(rad))
	pSetWindowRgn.Call(hwnd, rgn, 1)
	pShowWindow.Call(hwnd, SW_SHOW)
	pUpdateWindow.Call(hwnd)
	pSetForegroundWindow.Call(hwnd)
	pSetTimer.Call(hwnd, 1, 250, 0) // rafraîchit le statut / les actualités
	return true
}
