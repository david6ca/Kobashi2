//go:build appstore && darwin

package bridge

/*
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
int modelbridge_acquire_folder(const char *home, const char *client);
*/
import "C"
import (
	"unsafe"
)

func acquireClientAccess(home, client string) error {
	h, c := C.CString(home), C.CString(client)
	defer C.free(unsafe.Pointer(h))
	defer C.free(unsafe.Pointer(c))
	if C.modelbridge_acquire_folder(h, c) != 1 {
		return storeClientFolderError()
	}
	return nil
}
