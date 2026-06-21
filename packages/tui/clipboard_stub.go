//go:build !darwin || !cgo

package tui

func ReadClipboardImagePNG() (string, []byte, bool, error) {
	return "", nil, false, nil
}

func ReadClipboardText() (string, bool, error) {
	return "", false, nil
}
