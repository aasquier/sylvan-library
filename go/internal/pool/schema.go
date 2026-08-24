package pool

import _ "embed"

// Schema is the DDL that creates a pool, verbatim -- the same DDL every
// existing pool file was created under, so the file `data refresh` creates
// is the file the deployed volume already holds, and so the tests here can
// build a real pool from nothing.
//
//go:embed schema.sql
var Schema string
