package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"modernc.org/sqlite"

	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

const sqliteInitSQL = `
	PRAGMA journal_mode = WAL;
	PRAGMA synchronous = NORMAL;
	PRAGMA temp_store = MEMORY;
	PRAGMA mmap_size = 30000000000; -- 30GB
	PRAGMA busy_timeout = 5000;
	PRAGMA automatic_index = true;
	PRAGMA foreign_keys = ON;
	PRAGMA analysis_limit = 1000;
	PRAGMA trusted_schema = OFF;
`

type SQLiteDriver struct {
	Path string
}

type SQLiteDriverConnection struct {
	Read  *sql.DB
	Write *sql.DB
}

func NewSQLiteDriver() *SQLiteDriver {
	return &SQLiteDriver{}
}

func (d *SQLiteDriver) CreateDatabase(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	return nil
}

// TODO: add this to all drivers and call it once per app lifecycle
func (d *SQLiteDriver) Initialize(ctx context.Context) error {
	// this can only happen once
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
		_, err := conn.ExecContext(ctx, sqliteInitSQL, nil)

		return err
	})

	return nil
}

func (d *SQLiteDriver) Connect(ctx context.Context, config config.DriverConfig) (plugin.DriverConnection, error) {
	path := config.String("path")

	write, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return &SQLiteDriverConnection{}, err
	}
	// single writer ever, no concurrency
	write.SetMaxOpenConns(1)
	write.SetConnMaxIdleTime(time.Minute)
	if err != nil {
		return &SQLiteDriverConnection{}, err
	}

	read, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return &SQLiteDriverConnection{}, err
	}
	// readers can be concurrent
	read.SetMaxOpenConns(25)
	read.SetConnMaxIdleTime(time.Minute)

	conn := &SQLiteDriverConnection{
		Write: write,
		Read:  read,
	}
	return conn, nil
}

func (d *SQLiteDriver) Disconnect(ctx context.Context, conn plugin.DriverConnection) error {
	driverConn := conn.(*SQLiteDriverConnection)
	readErr := driverConn.Read.Close()
    writeErr := driverConn.Write.Close()
	if readErr != nil {
		return fmt.Errorf("failed to disconnect from SQLite(read) database: %w", readErr)
	}
	if writeErr != nil {
		return fmt.Errorf("failed to disconnect from SQLite(write) database: %w", writeErr)
	}
	return nil
}

func (c *SQLiteDriverConnection) LockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	return fmt.Errorf("not implemented")
}

func (c *SQLiteDriverConnection) UpsertAuditLogTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	return fmt.Errorf("not implemented")
}

func (c *SQLiteDriverConnection) UpsertAuditLockTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	return fmt.Errorf("not implemented")
}

func (c *SQLiteDriverConnection) AppendAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, log plugin.DriverAuditLog) error {
	return fmt.Errorf("not implemented")
}

func (c *SQLiteDriverConnection) ReadAuditLogs(ctx context.Context, execCtx plugin.DriverExecutionContext) ([]plugin.DriverAuditLog, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *SQLiteDriverConnection) UnlockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	return fmt.Errorf("not implemented")
}

func (c *SQLiteDriverConnection) Query(execCtx plugin.DriverExecutionContext) plugin.DriverQuery {
	return &SQLiteDriverQuery{}
}

type SQLiteDriverQuery struct{}

func (q *SQLiteDriverQuery) Exec(ctx context.Context, query string, args ...any) error {
	return fmt.Errorf("not implemented")
}

func (q *SQLiteDriverQuery) Query(ctx context.Context, query string, args ...any) (*plugin.DriverQueryResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *SQLiteDriverQuery) Begin(ctx context.Context) (plugin.DriverQuery, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *SQLiteDriverQuery) Commit(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (q *SQLiteDriverQuery) Rollback(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}
