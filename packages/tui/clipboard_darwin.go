//go:build darwin && cgo

package tui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

static char* zot_strdup(const char *s) {
	if (s == NULL) return NULL;
	char *out = malloc(strlen(s) + 1);
	if (out != NULL) strcpy(out, s);
	return out;
}

static int ZotWriteClipboardPNG(const char *path, char **err) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		NSArray *classes = @[[NSImage class]];
		NSDictionary *options = @{};
		NSArray *objects = [pb readObjectsForClasses:classes options:options];
		NSImage *image = nil;
		if ([objects count] > 0 && [[objects objectAtIndex:0] isKindOfClass:[NSImage class]]) {
			image = (NSImage *)[objects objectAtIndex:0];
		}
		if (image == nil) {
			NSData *tiff = [pb dataForType:NSPasteboardTypeTIFF];
			if (tiff != nil) image = [[NSImage alloc] initWithData:tiff];
		}
		if (image == nil) return 1;

		NSData *tiff = [image TIFFRepresentation];
		if (tiff == nil) {
			if (err != NULL) *err = zot_strdup("clipboard image has no TIFF representation");
			return 2;
		}
		NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
		if (rep == nil) {
			if (err != NULL) *err = zot_strdup("clipboard image could not be converted to a bitmap");
			return 2;
		}
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (png == nil) {
			if (err != NULL) *err = zot_strdup("clipboard image could not be encoded as PNG");
			return 2;
		}
		NSString *dst = [NSString stringWithUTF8String:path];
		NSError *writeErr = nil;
		if (![png writeToFile:dst options:NSDataWritingAtomic error:&writeErr]) {
			if (err != NULL) *err = zot_strdup([[writeErr localizedDescription] UTF8String]);
			return 2;
		}
		return 0;
	}
}
*/
import "C"

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

func ReadClipboardImagePNG() (string, []byte, bool, error) {
	dir := clipboardImageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, false, err
	}
	path := filepath.Join(dir, "clipboard-"+time.Now().Format("20060102-150405")+"-"+randomHex(4)+".png")
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var cerr *C.char
	code := C.ZotWriteClipboardPNG(cpath, &cerr)
	if cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
	}
	if code == 1 {
		return "", nil, false, nil
	}
	if code != 0 {
		msg := "could not read image from clipboard"
		if cerr != nil {
			msg = C.GoString(cerr)
		}
		return "", nil, false, fmt.Errorf("%s", msg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, false, err
	}
	return path, data, true, nil
}

func clipboardImageDir() string {
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		return filepath.Join("/tmp", "zot-clipboard-images")
	}
	return filepath.Join(os.TempDir(), "zot-clipboard-images")
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
