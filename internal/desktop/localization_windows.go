//go:build windows

package desktop

import (
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/david6ca/Kobashi2/internal/localization"
)

var preferredWindowsLanguages = func() []string {
	var count, length uint32
	api := kernel.NewProc("GetUserPreferredUILanguages")
	api.Call(8, uintptr(unsafe.Pointer(&count)), 0, uintptr(unsafe.Pointer(&length)))
	if length > 0 {
		buffer := make([]uint16, length)
		if ok, _, _ := api.Call(8, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&length))); ok != 0 {
			var result []string
			start := 0
			for i, unit := range buffer {
				if unit == 0 {
					if i > start {
						result = append(result, syscall.UTF16ToString(buffer[start:i]))
					}
					start = i + 1
				}
			}
			return result
		}
	}
	return []string{"en"}
}()

func (w *Window) locale() string {
	return localization.Resolve(text(w.status["language"]), preferredWindowsLanguages)
}
func (w *Window) tr(source string) string { return localization.Text(w.locale(), source) }
func (w *Window) title(source string) string {
	if w.locale() == "en" {
		return source
	}
	return w.tr(strings.ReplaceAll(source, "&", ""))
}

func (w *Window) localize() {
	locale := w.locale()
	if w.renderedLanguage == locale {
		return
	}
	w.renderedLanguage = locale
	for i := range w.controls {
		c := &w.controls[i]
		if c.view == "account-popup" || c.class == "EDIT" || c.title == "" {
			continue
		}
		w.setText(c.id, w.title(c.title))
	}
	selector := w.control(idAppearance)
	proc("SendMessageW", selector, 0x14b, 0, 0)
	for _, title := range []string{"Auto", "Light", "Dark"} {
		proc("SendMessageW", selector, 0x143, 0, uintptr(unsafe.Pointer(wide(w.tr(title)))))
	}
	proc("SendMessageW", w.control(idLanguage), 0x14b, 0, 0)
	for _, option := range append([]localization.Language{{Code: "auto", Name: w.tr("System Default")}}, localization.Languages()...) {
		proc("SendMessageW", w.control(idLanguage), 0x143, 0, uintptr(unsafe.Pointer(wide(option.Name))))
	}
	proc("SendMessageW", w.control(idLanguage), 0x1701, 14, 0)
	proc("SendMessageW", w.control(idToken), 0x1501, 1, uintptr(unsafe.Pointer(wide(w.tr("Paste your access token")))))
	// Mirror native child layout for Arabic; GDI bitmap orientation is retained.
	style := proc("GetWindowLongPtrW", w.Handle, ^uintptr(19))
	if locale == "ar" {
		style |= 0x400000
	} else {
		style &^= 0x400000
	}
	proc("SetWindowLongPtrW", w.Handle, ^uintptr(19), style)
	proc("SetWindowPos", w.Handle, 0, 0, 0, 0, 0, 0x37)
}

func (w *Window) showPrivacy() {
	flags := uintptr(0x40)
	if w.locale() == "ar" {
		flags |= 0x180000
	}
	proc("MessageBoxW", w.Handle, uintptr(unsafe.Pointer(wide(strings.ReplaceAll(localization.Privacy(w.locale()), "\n", "\r\n")))), uintptr(unsafe.Pointer(wide(w.tr("Privacy Statement")))), flags)
}

func (w *Window) credits(value float64) string {
	locale := map[string]string{"en": "en-US", "zh-Hans": "zh-CN", "zh-Hant": "zh-TW", "ja": "ja-JP", "ko": "ko-KR", "de": "de-DE", "fr": "fr-FR", "pt-BR": "pt-BR", "es": "es-ES", "ar": "ar-SA", "hi": "hi-IN", "ru": "ru-RU", "vi": "vi-VN"}[w.locale()]
	number := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 3, 64), "0"), ".")
	info := func(kind uintptr) []uint16 {
		buffer := make([]uint16, 32)
		kernel.NewProc("GetLocaleInfoEx").Call(uintptr(unsafe.Pointer(wide(locale))), kind, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		return buffer
	}
	decimal, thousands := info(0xe), info(0xf)
	grouping := uint32(0)
	for _, group := range strings.Split(syscall.UTF16ToString(info(0x10)), ";") {
		n, _ := strconv.Atoi(group)
		if n == 0 {
			break
		}
		grouping = grouping*10 + uint32(n)
	}
	var digits uint32
	if at := strings.IndexByte(number, '.'); at >= 0 {
		digits = uint32(len(number) - at - 1)
	}
	format := struct {
		Digits, LeadingZero, Grouping uint32
		Decimal, Thousands            *uint16
		NegativeOrder                 uint32
	}{digits, 1, grouping, &decimal[0], &thousands[0], 1}
	buffer := make([]uint16, 128)
	if size, _, _ := kernel.NewProc("GetNumberFormatEx").Call(uintptr(unsafe.Pointer(wide(locale))), 0, uintptr(unsafe.Pointer(wide(number))), uintptr(unsafe.Pointer(&format)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))); size > 0 {
		return syscall.UTF16ToString(buffer)
	}
	return credits(value)
}
