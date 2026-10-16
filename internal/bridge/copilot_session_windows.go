package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var copilotKernel32 = syscall.NewLazyDLL("kernel32.dll")

func copilotConnectionRefused(err error) bool {
	const wsaeConnRefused syscall.Errno = 10061
	return errors.Is(err, wsaeConnRefused)
}

func copilotQuery(path, sql string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	var systemDirectory [syscall.MAX_PATH]uint16
	size, _, err := copilotKernel32.NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&systemDirectory[0])), uintptr(len(systemDirectory)))
	if size == 0 {
		return nil, fmt.Errorf("locate Windows SQLite: %w", err)
	}
	if size >= uintptr(len(systemDirectory)) {
		return nil, errors.New("Windows system directory exceeds MAX_PATH")
	}
	sqlite := syscall.NewLazyDLL(filepath.Join(syscall.UTF16ToString(systemDirectory[:size]), "winsqlite3.dll"))
	if err := sqlite.Load(); err != nil {
		return nil, fmt.Errorf("Windows SQLite API is unavailable: %w", err)
	}
	defer syscall.FreeLibrary(syscall.Handle(sqlite.Handle()))
	for _, name := range []string{"sqlite3_open_v2", "sqlite3_close", "sqlite3_prepare_v2", "sqlite3_step",
		"sqlite3_column_text", "sqlite3_column_bytes", "sqlite3_finalize", "sqlite3_busy_timeout"} {
		if err := sqlite.NewProc(name).Find(); err != nil {
			return nil, fmt.Errorf("Windows SQLite API is unavailable: %w", err)
		}
	}
	file, err := syscall.BytePtrFromString(path)
	if err != nil {
		return nil, err
	}
	var db uintptr
	code, _, _ := sqlite.NewProc("sqlite3_open_v2").Call(uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(&db)), 1, 0)
	if db != 0 {
		defer sqlite.NewProc("sqlite3_close").Call(db)
	}
	if code != 0 {
		return nil, fmt.Errorf("open Copilot database read-only: SQLite status %d", code)
	}
	sqlite.NewProc("sqlite3_busy_timeout").Call(db, 2000)
	query, err := syscall.BytePtrFromString(sql)
	if err != nil {
		return nil, err
	}
	var statement uintptr
	code, _, _ = sqlite.NewProc("sqlite3_prepare_v2").Call(db, uintptr(unsafe.Pointer(query)), ^uintptr(0), uintptr(unsafe.Pointer(&statement)), 0)
	if code != 0 {
		return nil, fmt.Errorf("prepare Copilot session selection: SQLite status %d; the app may be busy or incompatible", code)
	}
	defer sqlite.NewProc("sqlite3_finalize").Call(statement)
	code, _, _ = sqlite.NewProc("sqlite3_step").Call(statement)
	if code == 101 {
		return nil, nil
	}
	if code != 100 {
		return nil, fmt.Errorf("read Copilot session: SQLite status %d", code)
	}
	size, _, _ = sqlite.NewProc("sqlite3_column_bytes").Call(statement, 0)
	if size > 16<<20 {
		return nil, errors.New("Copilot session data exceeds 16 MiB")
	}
	data, _, _ := sqlite.NewProc("sqlite3_column_text").Call(statement, 0)
	if data == 0 {
		return nil, errors.New("Copilot session selection is not text")
	}
	result := make([]byte, int(size))
	if size != 0 {
		// SQLite owns the source pointer; copy it through the native ABI.
		copilotKernel32.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&result[0])), data, size)
	}
	return result, nil
}
