package ui

import "github.com/aymanbagabas/go-nativeclipboard"

func ClipboardWrite(data []byte) {
	_, err := nativeclipboard.Text.Write(data)
	if err != nil {
		// do nothing
	}
}

func ClipboardWriteString(data string) {
    ClipboardWrite([]byte(data))
}
