//go:build darwin && cgo

package tui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

static char* ZotReadClipboardString(void) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		NSString *str = [pb stringForType:NSPasteboardTypeString];
		if (str == nil) return NULL;
		const char *utf8 = [str UTF8String];
		if (utf8 == NULL) return NULL;
		char *out = malloc(strlen(utf8) + 1);
		if (out != NULL) strcpy(out, utf8);
		return out;
	}
}
*/
import "C"

import "unsafe"

func ReadClipboardText() (string, bool, error) {
	cstr := C.ZotReadClipboardString()
	if cstr == nil {
		return "", false, nil
	}
	defer C.free(unsafe.Pointer(cstr))
	return C.GoString(cstr), true, nil
}
