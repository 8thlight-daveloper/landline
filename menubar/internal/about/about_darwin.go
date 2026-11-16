//go:build darwin

package about

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "about_darwin.h"
*/
import "C"

import (
	"unsafe"

	"github.com/8thlight-daveloper/landline/menubar/internal/icon"
)

func UseAccessoryPolicy() { C.LandlineUseAccessoryPolicy() }

func Show(content Content) {
	name, version := C.CString(content.Name), C.CString(content.Version)
	credit, link := C.CString(content.CreditsLine), C.CString(content.Link)
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(version))
	defer C.free(unsafe.Pointer(credit))
	defer C.free(unsafe.Pointer(link))
	C.LandlineShowAbout(name, version, unsafe.Pointer(&icon.About[0]), C.int(len(icon.About)), credit, link)
}
