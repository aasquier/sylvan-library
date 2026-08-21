package pool

import _ "embed"

// Schema is `cards/db.py:SCHEMA`, verbatim -- the DDL that creates a pool.
// Written by `tests/go_fixtures.py` and held equal to Python's by
// `tests/test_go_fixtures.py`, so the file the Go `data refresh` creates at
// Phase 8 is the file Python creates today, and so the tests here can build
// a real pool without a Python process anywhere near them.
//
//go:embed schema.sql
var Schema string
