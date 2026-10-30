//go:build linux && amd64

package duckdbthin

/*
#cgo CFLAGS: -I../../deps/linux_amd64
#cgo LDFLAGS: -L../../deps/linux_amd64 -lduckdb
*/
import "C"
