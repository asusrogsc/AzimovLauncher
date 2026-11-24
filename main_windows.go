// Azimov Online — launcher (Windows, Go sans dépendance).
//
// Compilation : voir build.bat (ou README.md).
package main

import (
	_ "embed"
	"runtime"
	"unsafe"
)

//go:embed assets/fond.jpg
var imgFond []byte

//go:embed assets/logo.png
var imgLogo []byte

//go:embed assets/Cinzel.ttf
var fontCinzel []byte

//go:embed assets/CinzelAZBold.ttf
var fontCinzelBold []byte

//go:embed assets/Marcellus.ttf
var fontMarcellus []byte

func init() {
	// la fenêtre, GDI+ et la boîte « Choisir un dossier » restent sur le même fil
	runtime.LockOSThread()
}

func main() {
	// netteté sur les écrans à 125 % / 150 % …
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	} else {
		pSetProcessDPIAware.Call()
	}
	pCoInitializeEx.Call(0, 0x2) // STA (boîte de dialogue des dossiers)

	if !gdipStart() {
		msgBox(0, "GDI+ n'a pas pu démarrer.", "Azimov Online", MB_ICONERROR)
		return
	}
	loadIni()

	scale := float32(1)
	if pGetDpiForSystem.Find() == nil {
		if dpi, _, _ := pGetDpiForSystem.Call(); dpi > 0 {
			scale = float32(dpi) / 96
		}
	}
	// la fenêtre doit tenir à l'écran
	sw, _, _ := pGetSystemMetrics.Call(SM_CXSCREEN)
	sh, _, _ := pGetSystemMetrics.Call(SM_CYSCREEN)
	for scale > 0.6 && (float32(baseW)*scale > float32(sw)*0.96 || float32(baseH)*scale > float32(sh)*0.92) {
		scale -= 0.05
	}

	u := &UI{
		scale:   scale,
		bgImg:   loadImage(imgFond),
		logoImg: loadImage(imgLogo),
		fonts:   newFontCollection(fontCinzel, fontCinzelBold, fontMarcellus),
		fam:     map[string]uintptr{},
		fcache:  map[string]uintptr{},
	}
	ui = u
	u.eng = NewEngine(func() {
		if u.hwnd != 0 {
			pPostMessageW.Call(u.hwnd, WM_APP, 0, 0)
		}
	})
	if !createWindow(u) {
		msgBox(0, "Impossible de créer la fenêtre.", "Azimov Online", MB_ICONERROR)
		return
	}
	u.eng.StartServerInfo()
	u.eng.Start()

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
