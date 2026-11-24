// Liaisons Win32 / GDI+ minimales (aucune dépendance externe).
package main

import (
	"math"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	shlwapi  = syscall.NewLazyDLL("shlwapi.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	gdiplus  = syscall.NewLazyDLL("gdiplus.dll")

	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pUpdateWindow                  = user32.NewProc("UpdateWindow")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pBeginPaint                    = user32.NewProc("BeginPaint")
	pEndPaint                      = user32.NewProc("EndPaint")
	pInvalidateRect                = user32.NewProc("InvalidateRect")
	pGetClientRect                 = user32.NewProc("GetClientRect")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pSetCursor                     = user32.NewProc("SetCursor")
	pLoadIconW                     = user32.NewProc("LoadIconW")
	pSetTimer                      = user32.NewProc("SetTimer")
	pTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	pScreenToClient                = user32.NewProc("ScreenToClient")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pMessageBoxW                   = user32.NewProc("MessageBoxW")
	pSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pCreateRoundRectRgn     = gdi32.NewProc("CreateRoundRectRgn")

	pGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	pGetDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")

	pShellExecuteW                = shell32.NewProc("ShellExecuteW")
	pSHBrowseForFolderW           = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW         = shell32.NewProc("SHGetPathFromIDListW")
	pSHCreateMemStream            = shlwapi.NewProc("SHCreateMemStream")
	pCoInitializeEx               = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree                = ole32.NewProc("CoTaskMemFree")
	pSetWindowRgn                 = user32.NewProc("SetWindowRgn")
	pGdiplusStartup               = gdiplus.NewProc("GdiplusStartup")
	pGdipCreateFromHDC            = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics           = gdiplus.NewProc("GdipDeleteGraphics")
	pGdipSetSmoothingMode         = gdiplus.NewProc("GdipSetSmoothingMode")
	pGdipSetTextRenderingHint     = gdiplus.NewProc("GdipSetTextRenderingHint")
	pGdipSetInterpolationMode     = gdiplus.NewProc("GdipSetInterpolationMode")
	pGdipSetPixelOffsetMode       = gdiplus.NewProc("GdipSetPixelOffsetMode")
	pGdipScaleWorldTransform      = gdiplus.NewProc("GdipScaleWorldTransform")
	pGdipCreateBitmapFromStream   = gdiplus.NewProc("GdipCreateBitmapFromStream")
	pGdipCreateBitmapFromScan0    = gdiplus.NewProc("GdipCreateBitmapFromScan0")
	pGdipGetImageGraphicsContext  = gdiplus.NewProc("GdipGetImageGraphicsContext")
	pGdipGetImageWidth            = gdiplus.NewProc("GdipGetImageWidth")
	pGdipGetImageHeight           = gdiplus.NewProc("GdipGetImageHeight")
	pGdipDisposeImage             = gdiplus.NewProc("GdipDisposeImage")
	pGdipDrawImageRect            = gdiplus.NewProc("GdipDrawImageRect")
	pGdipCreateSolidFill          = gdiplus.NewProc("GdipCreateSolidFill")
	pGdipCreateLineBrush          = gdiplus.NewProc("GdipCreateLineBrush")
	pGdipDeleteBrush              = gdiplus.NewProc("GdipDeleteBrush")
	pGdipCreatePen1               = gdiplus.NewProc("GdipCreatePen1")
	pGdipDeletePen                = gdiplus.NewProc("GdipDeletePen")
	pGdipFillRectangle            = gdiplus.NewProc("GdipFillRectangle")
	pGdipDrawLine                 = gdiplus.NewProc("GdipDrawLine")
	pGdipCreatePath               = gdiplus.NewProc("GdipCreatePath")
	pGdipDeletePath               = gdiplus.NewProc("GdipDeletePath")
	pGdipAddPathArc               = gdiplus.NewProc("GdipAddPathArc")
	pGdipAddPathEllipse           = gdiplus.NewProc("GdipAddPathEllipse")
	pGdipAddPathPolygon           = gdiplus.NewProc("GdipAddPathPolygon")
	pGdipClosePathFigure          = gdiplus.NewProc("GdipClosePathFigure")
	pGdipFillPath                 = gdiplus.NewProc("GdipFillPath")
	pGdipDrawPath                 = gdiplus.NewProc("GdipDrawPath")
	pGdipNewPrivateFontCollection = gdiplus.NewProc("GdipNewPrivateFontCollection")
	pGdipPrivateAddMemoryFont     = gdiplus.NewProc("GdipPrivateAddMemoryFont")
	pGdipCreateFontFamilyFromName = gdiplus.NewProc("GdipCreateFontFamilyFromName")
	pGdipCreateFont               = gdiplus.NewProc("GdipCreateFont")
	pGdipCreateStringFormat       = gdiplus.NewProc("GdipCreateStringFormat")
	pGdipSetStringFormatAlign     = gdiplus.NewProc("GdipSetStringFormatAlign")
	pGdipSetStringFormatLineAlign = gdiplus.NewProc("GdipSetStringFormatLineAlign")
	pGdipSetStringFormatTrimming  = gdiplus.NewProc("GdipSetStringFormatTrimming")
	pGdipSetStringFormatFlags     = gdiplus.NewProc("GdipSetStringFormatFlags")
	pGdipDeleteStringFormat       = gdiplus.NewProc("GdipDeleteStringFormat")
	pGdipDrawString               = gdiplus.NewProc("GdipDrawString")
	pGdipMeasureString            = gdiplus.NewProc("GdipMeasureString")
)

const (
	WS_POPUP             = 0x80000000
	WS_MINIMIZEBOX       = 0x00020000
	WS_SYSMENU           = 0x00080000
	WS_EX_APPWINDOW      = 0x00040000
	CS_HREDRAW           = 0x0002
	CS_VREDRAW           = 0x0001
	SW_SHOW              = 5
	SW_MINIMIZE          = 6
	SW_SHOWNORMAL        = 1
	WM_DESTROY           = 0x0002
	WM_PAINT             = 0x000F
	WM_CLOSE             = 0x0010
	WM_ERASEBKGND        = 0x0014
	WM_SETCURSOR         = 0x0020
	WM_NCHITTEST         = 0x0084
	WM_TIMER             = 0x0113
	WM_MOUSEMOVE         = 0x0200
	WM_LBUTTONDOWN       = 0x0201
	WM_LBUTTONUP         = 0x0202
	WM_MOUSELEAVE        = 0x02A3
	WM_APP               = 0x8000
	HTCLIENT             = 1
	HTCAPTION            = 2
	IDC_ARROW            = 32512
	IDC_HAND             = 32649
	SM_CXSCREEN          = 0
	SM_CYSCREEN          = 1
	SRCCOPY              = 0x00CC0020
	TME_LEAVE            = 0x00000002
	MB_OK                = 0x0
	MB_ICONERROR         = 0x10
	MB_ICONWARNING       = 0x30
	MB_YESNO             = 0x4
	IDYES                = 6
	BIF_RETURNONLYFSDIRS = 0x1
	BIF_NEWDIALOGSTYLE   = 0x40
)

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}

type point struct{ X, Y int32 }

type msgT struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	_       uint32
}

type rect struct{ Left, Top, Right, Bottom int32 }

type paintStruct struct {
	Hdc         uintptr
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RGBReserved [32]byte
}

type trackMouseEvent struct {
	Size, Flags uint32
	Hwnd        uintptr
	HoverTime   uint32
}

type browseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Fn          uintptr
	LParam      uintptr
	Image       int32
}

type gdiplusStartupInput struct {
	Version                  uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type rectF struct{ X, Y, W, H float32 }

func f32(v float32) uintptr { return uintptr(math.Float32bits(v)) }

func wstr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) int32 { return int32(int16(v & 0xFFFF)) }
func hiword(v uintptr) int32 { return int32(int16((v >> 16) & 0xFFFF)) }

func msgBox(hwnd uintptr, text, title string, flags uintptr) int {
	r, _, _ := pMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(wstr(text))), uintptr(unsafe.Pointer(wstr(title))), flags)
	return int(r)
}

func openURL(u string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(wstr("open"))), uintptr(unsafe.Pointer(wstr(u))), 0, 0, SW_SHOWNORMAL)
}

func diskFree(dir string) (uint64, bool) {
	var free uint64
	r, _, _ := pGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(wstr(dir))), uintptr(unsafe.Pointer(&free)), 0, 0)
	return free, r != 0
}

func browseFolder(owner uintptr, title string) string {
	buf := make([]uint16, 260)
	bi := browseInfo{Owner: owner, DisplayName: &buf[0], Title: wstr(title), Flags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(pidl)
	path := make([]uint16, 1024)
	if r, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(path)
}
