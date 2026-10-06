package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Advisory-lock keys. Arbitrary but fixed: every replica must agree on them.
const (
	lockKeyMigrations   int64 = 0x666c6474_00000001 // "fldt" + 1
	lockKeyHousekeeping int64 = 0x666c6474_00000002
)

// TryLock takes a session-level advisory lock on its own pooled connection,
// without waiting. ok=false means another replica holds it. The returned
// release must be called when done; it unlocks and returns the connection.
func TryLock(ctx context.Context, pool *pgxpool.Pool, key int64) (release func(), ok bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire lock connection: %w", err)
	}
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("try advisory lock: %w", err)
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, key)
		conn.Release()
	}, true, nil
}

// HousekeepingLock is TryLock for the worker's periodic maintenance, so that
// with several API replicas only one runs schedules, retention and alerts.
func HousekeepingLock(pool *pgxpool.Pool) func(context.Context) (func(), bool, error) {
	return func(ctx context.Context) (func(), bool, error) {
		return TryLock(ctx, pool, lockKeyHousekeeping)
	}
}

// lock blocks until the advisory lock is held.
func lock(ctx context.Context, pool *pgxpool.Pool, key int64) (func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lock connection: %w", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		conn.Release()
		return nil, fmt.Errorf("advisory lock: %w", err)
	}
	return func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, key)
		conn.Release()
	}, nil
}
