// Package duckdbthin 是 DuckDB 的薄 cgo 驱动（database/sql driver）。
//
// 设计动机：go-duckdb 官方驱动在 Windows 下依赖 MSVC 编译的静态库，
// 与 MinGW/其他工具链 ABI 不兼容。本驱动直接调用官方 duckdb.dll /
// libduckdb.so 的 C API（duckdb_open/connect/prepare/bind/execute/...），
// 只需替换动态库即可跨平台，代码零改动。
//
// 注册驱动名：duckdbthin（见 init）。
package duckdbthin

/*
#include <stdlib.h>
#include <string.h>
#include "duckdb.h"
*/
import "C"

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"math"
	"strings"
	"unsafe"
)

func init() {
	sql.Register("duckdbthin", &Driver{})
}

// Driver 实现 driver.Driver。
type Driver struct{}

// Open 打开（或创建）DuckDB 数据库。dsn 为空或 ":memory:" 表示内存库。
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	cpath := C.CString(dsn)
	defer C.free(unsafe.Pointer(cpath))

	var db C.duckdb_database
	if st := C.duckdb_open(cpath, &db); st == C.DuckDBError {
		// duckdb_open 失败时错误信息通过 duckdb_open_ext 才可读；这里给通用错误。
		return nil, fmt.Errorf("duckdb_open %q failed", dsn)
	}
	var conn C.duckdb_connection
	if st := C.duckdb_connect(db, &conn); st == C.DuckDBError {
		C.duckdb_close(&db)
		return nil, fmt.Errorf("duckdb_connect failed")
	}
	return &Conn{db: db, conn: conn}, nil
}

// Conn 实现 driver.Conn / driver.Execer / driver.Queryer / driver.ConnBeginTx。
type Conn struct {
	db   C.duckdb_database
	conn C.duckdb_connection
}

func (c *Conn) Close() error {
	if c.conn != nil {
		C.duckdb_disconnect(&c.conn)
		c.conn = nil
	}
	if c.db != nil {
		C.duckdb_close(&c.db)
		c.db = nil
	}
	return nil
}

func (c *Conn) Begin() (driver.Tx, error) { return c.BeginTx(context.Background(), driver.TxOptions{}) }

func (c *Conn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if _, err := c.Exec(`BEGIN`, nil); err != nil {
		return nil, err
	}
	return &Tx{conn: c}, nil
}

// Prepare 返回预编译语句（driver.Stmt）。
func (c *Conn) Prepare(query string) (driver.Stmt, error) {
	return c.prepare(query)
}

func (c *Conn) prepare(query string) (*Stmt, error) {
	cq := C.CString(query)
	defer C.free(unsafe.Pointer(cq))

	var ps C.duckdb_prepared_statement
	if st := C.duckdb_prepare(c.conn, cq, &ps); st == C.DuckDBError {
		msg := C.GoString(C.duckdb_prepare_error(ps))
		C.duckdb_destroy_prepare(&ps)
		return nil, fmt.Errorf("duckdb prepare: %s", msg)
	}
	return &Stmt{conn: c, ps: ps, numInput: int(C.duckdb_nparams(ps))}, nil
}

func (c *Conn) Exec(query string, args []driver.Value) (driver.Result, error) {
	st, err := c.prepare(query)
	if err != nil {
		// 多语句（如建表 Schema）无法 prepare 单条，改用 extract + 逐条 duckdb_query。
		if strings.Contains(err.Error(), "multiple statements") {
			return c.execMulti(query)
		}
		return nil, err
	}
	defer st.Close()
	return st.Exec(args)
}

// execMulti 用 duckdb_extract_statements 拆分多语句并逐条执行（仅用于无参数 DDL）。
func (c *Conn) execMulti(query string) (driver.Result, error) {
	cq := C.CString(query)
	defer C.free(unsafe.Pointer(cq))

	var ex C.duckdb_extracted_statements
	n := C.duckdb_extract_statements(c.conn, cq, &ex)
	defer C.duckdb_destroy_extracted(&ex)
	if n == 0 {
		return nil, fmt.Errorf("duckdb extract statements: no statements")
	}
	for i := C.idx_t(0); i < n; i++ {
		var ps C.duckdb_prepared_statement
		if st := C.duckdb_prepare_extracted_statement(c.conn, ex, i, &ps); st == C.DuckDBError {
			msg := C.GoString(C.duckdb_prepare_error(ps))
			C.duckdb_destroy_prepare(&ps)
			return nil, fmt.Errorf("duckdb prepare extracted #%d: %s", i, msg)
		}
		var res C.duckdb_result
		s := C.duckdb_execute_prepared(ps, &res)
		C.duckdb_destroy_prepare(&ps)
		if s == C.DuckDBError {
			msg := C.GoString(C.duckdb_result_error(&res))
			C.duckdb_destroy_result(&res)
			return nil, fmt.Errorf("duckdb execute extracted #%d: %s", i, msg)
		}
		C.duckdb_destroy_result(&res)
	}
	return driver.RowsAffected(0), nil
}

func (c *Conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	st, err := c.prepare(query)
	if err != nil {
		return nil, err
	}
	rows, err := st.Query(args)
	if err != nil {
		st.Close()
		return nil, err
	}
	// Rows 生命周期内保持 st 存活：让 Rows 持有 st，关闭时一起释放。
	rows.(*Rows).stmt = st
	return rows, nil
}

func (c *Conn) ResetSession() error { return nil }
func (c *Conn) IsValid() bool       { return c.conn != nil }

// Tx 实现 driver.Tx。
type Tx struct {
	conn *Conn
}

func (t *Tx) Commit() error   { _, err := t.conn.Exec(`COMMIT`, nil); return err }
func (t *Tx) Rollback() error { _, err := t.conn.Exec(`ROLLBACK`, nil); return err }

// Stmt 实现 driver.Stmt。
type Stmt struct {
	conn     *Conn
	ps       C.duckdb_prepared_statement
	numInput int
}

func (s *Stmt) Close() error {
	if s.ps != nil {
		C.duckdb_destroy_prepare(&s.ps)
		s.ps = nil
	}
	return nil
}

func (s *Stmt) NumInput() int { return s.numInput }

func (s *Stmt) Exec(args []driver.Value) (driver.Result, error) {
	if err := s.bind(args); err != nil {
		return nil, err
	}
	var res C.duckdb_result
	if st := C.duckdb_execute_prepared(s.ps, &res); st == C.DuckDBError {
		msg := C.GoString(C.duckdb_result_error(&res))
		C.duckdb_destroy_result(&res)
		return nil, fmt.Errorf("duckdb execute: %s", msg)
	}
	n := int64(C.duckdb_row_count(&res))
	C.duckdb_destroy_result(&res)
	return driver.RowsAffected(n), nil
}

func (s *Stmt) Query(args []driver.Value) (driver.Rows, error) {
	if err := s.bind(args); err != nil {
		return nil, err
	}
	var res C.duckdb_result
	if st := C.duckdb_execute_prepared(s.ps, &res); st == C.DuckDBError {
		msg := C.GoString(C.duckdb_result_error(&res))
		C.duckdb_destroy_result(&res)
		return nil, fmt.Errorf("duckdb execute: %s", msg)
	}
	return &Rows{res: &res, ncols: int(C.duckdb_column_count(&res)), nrows: int(C.duckdb_row_count(&res))}, nil
}

// bind 将 args 绑定到 prepared statement（索引从 1 起）。
func (s *Stmt) bind(args []driver.Value) error {
	if len(args) != s.numInput {
		return fmt.Errorf("argument count mismatch: got %d, want %d", len(args), s.numInput)
	}
	for i, a := range args {
		idx := C.idx_t(i + 1)
		var st C.duckdb_state
		switch v := a.(type) {
		case nil:
			st = C.duckdb_bind_null(s.ps, idx)
		case bool:
			st = C.duckdb_bind_boolean(s.ps, idx, C.bool(v))
		case int:
			st = C.duckdb_bind_int64(s.ps, idx, C.int64_t(v))
		case int64:
			st = C.duckdb_bind_int64(s.ps, idx, C.int64_t(v))
		case int32:
			st = C.duckdb_bind_int32(s.ps, idx, C.int32_t(v))
		case int16:
			st = C.duckdb_bind_int16(s.ps, idx, C.int16_t(v))
		case int8:
			st = C.duckdb_bind_int8(s.ps, idx, C.int8_t(v))
		case uint64:
			if v <= math.MaxInt64 {
				st = C.duckdb_bind_int64(s.ps, idx, C.int64_t(v))
			} else {
				st = C.duckdb_bind_uint64(s.ps, idx, C.uint64_t(v))
			}
		case uint32:
			st = C.duckdb_bind_uint32(s.ps, idx, C.uint32_t(v))
		case uint16:
			st = C.duckdb_bind_uint16(s.ps, idx, C.uint16_t(v))
		case uint8:
			st = C.duckdb_bind_uint8(s.ps, idx, C.uint8_t(v))
		case float64:
			st = C.duckdb_bind_double(s.ps, idx, C.double(v))
		case float32:
			st = C.duckdb_bind_float(s.ps, idx, C.float(v))
		case string:
			cs := C.CString(v)
			st = C.duckdb_bind_varchar(s.ps, idx, cs)
			C.free(unsafe.Pointer(cs))
		default:
			return fmt.Errorf("unsupported arg type %T at index %d", a, i+1)
		}
		if st == C.DuckDBError {
			return fmt.Errorf("duckdb bind param %d failed", i+1)
		}
	}
	return nil
}

// Rows 实现 driver.Rows。
type Rows struct {
	res   *C.duckdb_result
	stmt  *Stmt // 非 nil 时由 Rows 负责释放 stmt
	pos   int
	ncols int
	nrows int
}

func (r *Rows) Columns() []string {
	cols := make([]string, r.ncols)
	for i := 0; i < r.ncols; i++ {
		cols[i] = C.GoString(C.duckdb_column_name(r.res, C.idx_t(i)))
	}
	return cols
}

func (r *Rows) Close() error {
	if r.res != nil {
		C.duckdb_destroy_result(r.res)
		r.res = nil
	}
	if r.stmt != nil {
		r.stmt.Close()
		r.stmt = nil
	}
	return nil
}

func (r *Rows) Next(dest []driver.Value) error {
	if r.pos >= r.nrows {
		return io.EOF
	}
	for i := range dest {
		col := C.idx_t(i)
		if C.duckdb_value_is_null(r.res, col, C.idx_t(r.pos)) == C.bool(true) {
			dest[i] = nil
			continue
		}
		switch C.duckdb_column_type(r.res, col) {
		case C.DUCKDB_TYPE_BOOLEAN:
			dest[i] = bool(C.duckdb_value_boolean(r.res, col, C.idx_t(r.pos)))
		case C.DUCKDB_TYPE_TINYINT, C.DUCKDB_TYPE_SMALLINT, C.DUCKDB_TYPE_INTEGER:
			dest[i] = int64(C.duckdb_value_int32(r.res, col, C.idx_t(r.pos)))
		case C.DUCKDB_TYPE_BIGINT:
			dest[i] = int64(C.duckdb_value_int64(r.res, col, C.idx_t(r.pos)))
		case C.DUCKDB_TYPE_UTINYINT, C.DUCKDB_TYPE_USMALLINT, C.DUCKDB_TYPE_UINTEGER:
			dest[i] = int64(C.duckdb_value_uint32(r.res, col, C.idx_t(r.pos)))
		case C.DUCKDB_TYPE_UBIGINT:
			dest[i] = int64(C.duckdb_value_uint64(r.res, col, C.idx_t(r.pos)))
		case C.DUCKDB_TYPE_FLOAT:
			dest[i] = float64(C.duckdb_value_float(r.res, col, C.idx_t(r.pos)))
		case C.DUCKDB_TYPE_DOUBLE:
			dest[i] = float64(C.duckdb_value_double(r.res, col, C.idx_t(r.pos)))
		default: // VARCHAR / 其余类型统一按字符串读
			// duckdb_value_varchar 返回 char*，须 duckdb_free。
			p := C.duckdb_value_varchar(r.res, col, C.idx_t(r.pos))
			dest[i] = C.GoString(p)
			C.duckdb_free(unsafe.Pointer(p))
		}
	}
	r.pos++
	return nil
}

var (
	_ driver.Driver      = (*Driver)(nil)
	_ driver.Conn        = (*Conn)(nil)
	_ driver.ConnBeginTx = (*Conn)(nil)
	_ driver.Execer      = (*Conn)(nil)
	_ driver.Queryer     = (*Conn)(nil)
	_ driver.Tx          = (*Tx)(nil)
	_ driver.Stmt        = (*Stmt)(nil)
	_ driver.Rows        = (*Rows)(nil)
)
