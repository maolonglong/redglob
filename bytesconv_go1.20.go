//go:build go1.20
// +build go1.20

package redglob

import "unsafe"

func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
