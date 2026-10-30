//go:build windows && amd64

package duckdbthin

/*
#cgo CFLAGS: -I../../deps/windows_amd64
#cgo LDFLAGS: -L../../deps/windows_amd64 -lduckdb
*/
import "C"
