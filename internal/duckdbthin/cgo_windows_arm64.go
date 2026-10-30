//go:build windows && arm64

package duckdbthin

/*
#cgo CFLAGS: -I../../deps/windows_arm64
#cgo LDFLAGS: -L../../deps/windows_arm64 -lduckdb
*/
import "C"
