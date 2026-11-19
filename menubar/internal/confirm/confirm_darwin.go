//go:build darwin

package confirm

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "confirm_darwin.h"
*/
import "C"

import "unsafe"

func Ask(question, accept string) bool {
	q, a, d := C.CString(question), C.CString(accept), C.CString("Cancel")
	defer C.free(unsafe.Pointer(q))
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(d))
	return C.LandlineConfirm(q, a, d) == 1
}
