package postgresstats

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Stats holds a snapshot of Postgres metrics for display.
type Stats struct {
	Connections    int     // current connections
	MaxConnections int     // max_connections
	IdleInTx       int     // idle in transaction (stuck)
	LongRunning    int     // active queries running > longQuerySec
	BlockingLocks  int     // backends waiting on locks
	CacheHitRatio  float64 // 0–1, from pg_stat_database
	DatabaseSize   string  // human-readable size
	Error          string  // non-empty if fetch failed
}

const longQuerySec = 30

// ensureSSLOption appends sslmode=disable to the URL if no sslmode is set,
// so local Postgres without SSL (common in dev) works by default.
func ensureSSLOption(url string) string {
	if strings.Contains(url, "sslmode=") {
		return url
	}
	if strings.Contains(url, "?") {
		return url + "&sslmode=disable"
	}
	return url + "?sslmode=disable"
}

// Fetch connects to the given Postgres URL, runs read-only queries, and returns Stats.
// It uses a short timeout so the TUI doesn't block.
// If the URL does not specify sslmode, sslmode=disable is added so local servers without SSL work.
func Fetch(ctx context.Context, url string) Stats {
	out := Stats{}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url = ensureSSLOption(url)
	db, err := sql.Open("postgres", url)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer db.Close()

	// Connections and max
	var maxConn int
	if err := db.QueryRowContext(ctx, "SHOW max_connections").Scan(&maxConn); err != nil {
		out.Error = "max_connections: " + err.Error()
		return out
	}
	out.MaxConnections = maxConn

	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity").Scan(&out.Connections); err != nil {
		out.Error = "connections: " + err.Error()
		return out
	}

	// Idle in transaction (stuck)
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE state = 'idle in transaction'",
	).Scan(&out.IdleInTx); err != nil {
		out.Error = "idle_in_tx: " + err.Error()
		return out
	}

	// Long-running active queries (> 30s)
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND (now() - query_start) > interval '30 seconds'",
	).Scan(&out.LongRunning); err != nil {
		out.Error = "long_running: " + err.Error()
		return out
	}

	// Blocking (waiting on locks)
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock'",
	).Scan(&out.BlockingLocks); err != nil {
		// wait_event_type exists in PG 10+; ignore error and leave 0
		_ = err
	}

	// Cache hit ratio for current DB
	var hit, read int64
	if err := db.QueryRowContext(ctx,
		"SELECT blks_hit, blks_read FROM pg_stat_database WHERE datname = current_database()",
	).Scan(&hit, &read); err == nil && (hit+read) > 0 {
		out.CacheHitRatio = float64(hit) / float64(hit+read)
	}

	// Database size
	var sizeBytes int64
	if err := db.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&sizeBytes); err == nil {
		out.DatabaseSize = formatSize(sizeBytes)
	}

	return out
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
