//go:build linux && arm64

package duckdbthin

/*
#cgo CFLAGS: -I../../deps/linux_arm64
#cgo LDFLAGS: -L../../deps/linux_arm64 -lduckdb
*/
import "C"
