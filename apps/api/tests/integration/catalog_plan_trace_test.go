//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

type catalogTraceKey struct{}
type catalogTraceStart struct {
	at              time.Time
	sql             string
	args            []any
	name            string
	generic, custom int64
}
type catalogPlanTrace struct {
	mu   sync.Mutex
	file string
}

// Opt-in isolated diagnostics only. Never record arguments, session hashes,
// credentials or non-catalog plans. A failed query remains FAIL even when a
// separate, longer diagnostic EXPLAIN completes. Never use this as acceptance.
func attachCatalogPlanTrace(t *testing.T, d *db.Database, dir string) {
	t.Helper()
	if !strings.HasPrefix(filepath.Clean(dir)+"/", "/artifacts/") {
		t.Fatal("catalog plan diagnostics require isolated /artifacts")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	cfg := d.Pool.Config()
	cfg.ConnConfig.Tracer = &catalogPlanTrace{file: filepath.Join(dir, "plans.jsonl")}
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	d.Pool.Close()
	d.Pool = pool
}

func (c *catalogPlanTrace) record(v any) {
	b, e := json.Marshal(v)
	if e != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	f, e := os.OpenFile(c.file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
}

func (c *catalogPlanTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if ctx.Value(catalogTraceKey{}) != nil || !strings.HasPrefix(data.SQL, "WITH r AS (") || !strings.Contains(data.SQL, "matched AS") || len(data.Args) != 5 {
		return ctx
	}
	query, ok := data.Args[0].(string)
	if !ok || query != "" {
		return ctx
	}
	inner := context.WithValue(ctx, catalogTraceKey{}, true)
	h := sha256.Sum256([]byte(data.SQL))
	r := map[string]any{"at": time.Now().UTC(), "pid": conn.PgConn().PID(), "query_sha256": hex.EncodeToString(h[:])}
	var name, jit, mode string
	_ = conn.QueryRow(inner, `SELECT current_setting('jit'),current_setting('plan_cache_mode')`).Scan(&jit, &mode)
	r["jit"], r["plan_cache_mode"] = jit, mode
	var generic, custom int64
	_ = conn.QueryRow(inner, `SELECT name,generic_plans,custom_plans FROM pg_prepared_statements WHERE statement=$1 LIMIT 1`, data.SQL).Scan(&name, &generic, &custom)
	r["generic_before"], r["custom_before"] = generic, custom
	if _, e := conn.Exec(inner, `SAVEPOINT catalog_plan_diagnostic`); e != nil {
		r["savepoint_error"] = true
		c.record(r)
		return ctx
	}
	c.record(r)
	return context.WithValue(ctx, catalogTraceKey{}, catalogTraceStart{at: time.Now(), sql: data.SQL, args: data.Args, name: name, generic: generic, custom: custom})
}

func (c *catalogPlanTrace) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(catalogTraceKey{}).(catalogTraceStart)
	if !ok {
		return
	}
	r := map[string]any{"at": time.Now().UTC(), "pid": conn.PgConn().PID(), "elapsed_ms": time.Since(start.at).Milliseconds(), "ok": data.Err == nil}
	inner := context.WithValue(ctx, catalogTraceKey{}, true)
	var pgerr *pgconn.PgError
	if errors.As(data.Err, &pgerr) {
		r["sqlstate"] = pgerr.Code
	}
	if data.Err == nil {
		_, _ = conn.Exec(inner, `RELEASE SAVEPOINT catalog_plan_diagnostic`)
		c.record(r)
		return
	}
	if pgerr == nil || pgerr.Code != "57014" {
		c.record(r)
		return
	}
	if _, e := conn.Exec(inner, `ROLLBACK TO SAVEPOINT catalog_plan_diagnostic`); e != nil {
		r["recovery_error"] = true
		c.record(r)
		return
	}
	var name string
	var generic, custom int64
	_ = conn.QueryRow(inner, `SELECT name,generic_plans,custom_plans FROM pg_prepared_statements WHERE statement=$1 LIMIT 1`, start.sql).Scan(&name, &generic, &custom)
	r["generic_after"], r["custom_after"] = generic, custom
	mode := "force_custom_plan"
	if generic > start.generic {
		mode = "force_generic_plan"
	}
	r["diagnostic_plan_mode"] = mode
	// This is a separate investigation AFTER the original timeout. The error
	// is returned unchanged to the caller. No acceptance deadline is increased.
	if _, e := conn.Exec(inner, `SET LOCAL statement_timeout='10s'; SET LOCAL plan_cache_mode=`+mode); e != nil {
		r["diagnostic_setup_error"] = true
		c.record(r)
		return
	}
	explain := "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) " + start.sql
	args := start.args
	if name != "" {
		filter, fOK := start.args[1].(string)
		org, oOK := start.args[2].(string)
		offset, aOK := start.args[3].(int)
		size, bOK := start.args[4].(int)
		if fOK && oOK && aOK && bOK {
			literal := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
			explain = fmt.Sprintf("EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) EXECUTE %s('',%s,%s,%d,%d)", pgx.Identifier{name}.Sanitize(), literal(filter), literal(org), offset, size)
			args = nil
		}
	}
	var plan json.RawMessage
	if e := conn.QueryRow(inner, explain, args...).Scan(&plan); e == nil {
		r["diagnostic_plan"] = plan
	} else {
		var p *pgconn.PgError
		if errors.As(e, &p) {
			r["diagnostic_sqlstate"] = p.Code
		} else {
			r["diagnostic_error"] = "unavailable"
		}
	}
	c.record(r)
}
