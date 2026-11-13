//go:build windows

package desktop

import (
	"errors"
	"syscall"
	"unsafe"
)

const accountControlBase = 2000

var accountPopupCallback uintptr
var accountClassRegistered bool

func (w *Window) showAccounts() error {
	if w.view == "starting" || w.view == "quitting" || w.accountPopup != 0 {
		return nil
	}
	if !accountClassRegistered {
		accountPopupCallback = syscall.NewCallback(accountPopupProc)
		class := windowClass{Procedure: accountPopupCallback, Name: wide("KobashiAccountPopup"),
			Cursor: proc("LoadCursorW", 0, 32512)}
		class.Size = uint32(unsafe.Sizeof(class))
		if proc("RegisterClassExW", uintptr(unsafe.Pointer(&class))) == 0 {
			return errors.New("could not register the accounts popup")
		}
		accountClassRegistered = true
	}
	w.popupAccounts = nil
	accounts, _ := w.status["accounts"].([]any)
	for _, value := range accounts {
		if account := object(value); account != nil {
			w.popupAccounts = append(w.popupAccounts, account)
		}
	}
	height := min(216, len(w.popupAccounts)*36)
	var anchor rect
	proc("GetWindowRect", w.control(idAccounts), uintptr(unsafe.Pointer(&anchor)))
	width, popupHeight := 284*w.dpi/96, (height+50)*w.dpi/96
	x, y := int(anchor.Right)-width, int(anchor.Bottom)
	monitor := struct {
		Size          uint32
		Monitor, Work rect
		Flags         uint32
	}{}
	monitor.Size = uint32(unsafe.Sizeof(monitor))
	if proc("GetMonitorInfoW", proc("MonitorFromWindow", w.Handle, 2), uintptr(unsafe.Pointer(&monitor))) != 0 {
		x = max(int(monitor.Work.Left), min(x, int(monitor.Work.Right)-width))
		y = max(int(monitor.Work.Top), min(y, int(monitor.Work.Bottom)-popupHeight))
	}
	w.accountOffset = 0
	w.accountPopup = proc("CreateWindowExW", 0x80, uintptr(unsafe.Pointer(wide("KobashiAccountPopup"))),
		uintptr(unsafe.Pointer(wide("Accounts"))), 0x80800000,
		uintptr(x), uintptr(y), uintptr(width), uintptr(popupHeight), w.Handle, 0, 0, 0)
	if w.accountPopup == 0 {
		return errors.New("could not open the accounts popup")
	}
	w.accountList = proc("CreateWindowExW", 0, uintptr(unsafe.Pointer(wide("KobashiAccountPopup"))), 0,
		0x50200000, 0, 0, uintptr(282*w.dpi/96), uintptr(height*w.dpi/96), w.accountPopup, 0, 0, 0)
	if w.accountList == 0 {
		w.closeAccounts()
		return errors.New("could not create the accounts list")
	}
	add := func(title string, id, x, y, width, h int, parent uintptr) error {
		if err := w.add("BUTTON", title, "account-popup", id, x, y, width, h, 0x10000); err != nil {
			return err
		}
		proc("SetParent", w.control(id), parent)
		return nil
	}
	for i, account := range w.popupAccounts {
		name := "@" + text(account["username"])
		label := w.tr("Switch to this account") + ": " + name
		if account["selected"] == true {
			label = name + ", " + w.tr("Current account")
		}
		if err := add(label, accountControlBase+i*2, 0, i*36, 228, 36, w.accountList); err != nil {
			w.closeAccounts()
			return err
		}
		if err := add(w.tr("Remove account")+": "+name, accountControlBase+i*2+1, 228, i*36, 36, 36, w.accountList); err != nil {
			w.closeAccounts()
			return err
		}
	}
	if err := add(w.tr("Add account"), idAddAccount, 8, height+9, 264, 32, w.accountPopup); err != nil {
		w.closeAccounts()
		return err
	}
	w.scrollAccounts(0)
	w.updateAccountControls()
	proc("ShowWindow", w.accountPopup, 5)
	proc("SetForegroundWindow", w.accountPopup)
	first := idAddAccount
	if len(w.popupAccounts) > 0 {
		first = accountControlBase
	}
	if w.busy {
		proc("SetFocus", w.accountPopup)
	} else {
		proc("SetFocus", w.control(first))
	}
	return nil
}

func (w *Window) updateAccountControls() {
	enabled := uintptr(1)
	if w.busy || w.view == "quitting" {
		enabled = 0
	}
	for _, c := range w.controls {
		if c.view == "account-popup" {
			proc("EnableWindow", c.handle, enabled)
		}
	}
}

func (w *Window) closeAccounts() {
	if w.accountPopup == 0 {
		return
	}
	popup := w.accountPopup
	w.accountPopup, w.accountList = 0, 0
	kept := w.controls[:0]
	for _, c := range w.controls {
		if c.view != "account-popup" {
			kept = append(kept, c)
		}
	}
	w.controls = kept
	proc("DestroyWindow", popup)
	proc("SetFocus", w.control(idAccounts))
}

func (w *Window) scrollAccounts(offset int) {
	height := min(216, len(w.popupAccounts)*36)
	w.accountOffset = max(0, min(offset, len(w.popupAccounts)*36-height))
	for _, c := range w.controls {
		if c.id >= accountControlBase {
			proc("SetWindowPos", c.handle, 0, uintptr(c.x*w.dpi/96), uintptr((c.baseY-w.accountOffset)*w.dpi/96), 0, 0, 0x15)
		}
	}
	info := struct {
		Size, Mask uint32
		Min, Max   int32
		Page       uint32
		Pos, Track int32
	}{Mask: 7, Max: int32(max(0, len(w.popupAccounts)*36-1)), Page: uint32(height), Pos: int32(w.accountOffset)}
	info.Size = uint32(unsafe.Sizeof(info))
	proc("SetScrollInfo", w.accountList, 1, uintptr(unsafe.Pointer(&info)), 1)
}

func (w *Window) accountCommand(id int) {
	if w.busy {
		return
	}
	index := (id - accountControlBase) / 2
	if id < accountControlBase || index >= len(w.popupAccounts) {
		return
	}
	username := text(w.popupAccounts[index]["username"])
	path := "/api/accounts/switch"
	if id%2 == 1 {
		path = "/api/accounts/delete"
	}
	w.closeAccounts()
	w.post(path, map[string]any{"username": username})
}

func (w *Window) redrawAccountRow(id int) {
	if id < accountControlBase {
		return
	}
	base := id - (id-accountControlBase)%2
	proc("InvalidateRect", w.control(base), 0, 0)
	proc("InvalidateRect", w.control(base+1), 0, 0)
}

func (w *Window) paintAccountControl(p painter, c control) {
	index := (c.id - accountControlBase) / 2
	if index >= len(w.popupAccounts) {
		return
	}
	base := accountControlBase + index*2
	highlighted := false
	for _, other := range w.controls {
		if other.id == base || other.id == base+1 {
			highlighted = highlighted || other.hover || proc("GetFocus") == other.handle
		}
	}
	colors := w.palette
	bg := colors.panel
	if highlighted {
		bg = colors.hover
	}
	p.fill(0, 0, c.w, c.h, bg)
	if c.id%2 == 1 {
		if highlighted {
			p.line(13, 13, 21, 21, 1, colors.muted)
			p.line(21, 13, 13, 21, 1, colors.muted)
		}
	} else {
		account := w.popupAccounts[index]
		if account["selected"] == true {
			p.line(9, 18, 13, 22, 2, colors.connected)
			p.line(13, 22, 20, 13, 2, colors.connected)
		}
		p.text("@"+text(account["username"]), 30, 0, c.w-34, c.h, w.small, colors.text, 0x8824)
	}
}

func accountPopupProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	w := active
	if w == nil {
		return proc("DefWindowProcW", hwnd, uintptr(msg), wp, lp)
	}
	switch msg {
	case 0x111:
		id := int(wp & 0xffff)
		if id >= accountControlBase {
			w.accountCommand(id)
		} else {
			w.command(id)
		}
		return 0
	case 6:
		if wp&0xffff == 0 && hwnd == w.accountPopup {
			w.closeAccounts()
		}
	case 0x115:
		offset := w.accountOffset
		switch wp & 0xffff {
		case 0:
			offset -= 36
		case 1:
			offset += 36
		case 2:
			offset -= 216
		case 3:
			offset += 216
		case 4, 5:
			offset = int(wp >> 16)
		}
		w.scrollAccounts(offset)
		return 0
	case 0x20a:
		w.scrollAccounts(w.accountOffset - int(int16(wp>>16))/120*36)
		return 0
	case 0xf:
		var ps paintStruct
		dc := proc("BeginPaint", hwnd, uintptr(unsafe.Pointer(&ps)))
		p := painter{w, dc}
		p.fill(0, 0, 284, 266, w.palette.panel)
		if hwnd == w.accountPopup {
			y := min(216, len(w.popupAccounts)*36) + 4
			p.line(8, y, 272, y, 1, w.palette.border)
		}
		proc("EndPaint", hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case 0x14:
		return 1
	}
	return proc("DefWindowProcW", hwnd, uintptr(msg), wp, lp)
}

func (w *Window) accountKey(msg message) bool {
	if w.accountPopup == 0 || msg.Message != 0x100 {
		return false
	}
	if msg.WParam == 27 {
		w.closeAccounts()
		return true
	}
	if w.busy {
		return msg.WParam == 13 || msg.WParam == 9 || msg.WParam == 38 || msg.WParam == 40
	}
	if msg.WParam == 13 {
		proc("SendMessageW", proc("GetFocus"), 0xf5, 0, 0)
		return true
	}
	if msg.WParam != 9 && msg.WParam != 38 && msg.WParam != 40 {
		return false
	}
	ids := []int{}
	for i := range w.popupAccounts {
		ids = append(ids, accountControlBase+i*2)
		if msg.WParam == 9 {
			ids = append(ids, accountControlBase+i*2+1)
		}
	}
	ids = append(ids, idAddAccount)
	index := len(ids) - 1
	focus := proc("GetFocus")
	for i, id := range ids {
		if w.control(id) == focus {
			index = i
		}
	}
	delta := 1
	if msg.WParam == 38 || (msg.WParam == 9 && proc("GetKeyState", 16)&0x8000 != 0) {
		delta = -1
	}
	id := ids[(index+delta+len(ids))%len(ids)]
	if id >= accountControlBase {
		y := (id - accountControlBase) / 2 * 36
		if y < w.accountOffset {
			w.scrollAccounts(y)
		} else if y+36 > w.accountOffset+216 {
			w.scrollAccounts(y + 36 - 216)
		}
	}
	proc("SendMessageW", w.Handle, 0x127, 0x10002, 0)
	proc("SetFocus", w.control(id))
	return true
}
