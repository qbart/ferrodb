package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func (c *SQLiteDriverConnection) UpsertAuditLogTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		sqliteColumnDefinition(&plugin.DriverAuditColumnID),
		sqliteColumnDefinition(&plugin.DriverAuditColumnAppliedAt),
		sqliteColumnDefinition(&plugin.DriverAuditColumnEvent),
		sqliteColumnDefinition(&plugin.DriverAuditColumnData),
		sqliteColumnDefinition(&plugin.DriverAuditColumnMetadata),
	}
	_, err := c.Write.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s)`,
			sqliteTableName(execCtx, plugin.DriverAuditLogTableName),
			strings.Join(columns, ","),
		),
	)
	return err
}

func (c *SQLiteDriverConnection) UpsertAuditLockTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		sqliteColumnDefinition(&plugin.DriverAuditLockColumnID),
		sqliteColumnDefinition(&plugin.DriverAuditLockColumnLockedAt),
		sqliteColumnDefinition(&plugin.DriverAuditLockColumnLockedBy),
		sqliteColumnDefinition(&plugin.DriverAuditLockColumnData),
	}
	_, err := c.Write.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s)`,
			sqliteTableName(execCtx, plugin.DriverAuditLockTableName),
			strings.Join(columns, ","),
		),
	)
	return err
}

func (c *SQLiteDriverConnection) AppendAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, log plugin.DriverAuditLog) error {
	data, err := json.Marshal(log.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	var metadataArg any
	if log.Metadata != nil {
		b, err := json.Marshal(log.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metadataArg = string(b)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?)`,
		sqliteTableName(execCtx, plugin.DriverAuditLogTableName),
		sqliteQuoteIdent(plugin.DriverAuditColumnID.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnData.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
	)
	_, err = c.Write.ExecContext(ctx, query,
		log.ID,
		log.AppliedAt.Format(time.RFC3339Nano),
		log.Event,
		string(data),
		metadataArg,
	)
	return err
}

func (c *SQLiteDriverConnection) ReadAuditLogs(ctx context.Context, execCtx plugin.DriverExecutionContext) ([]plugin.DriverAuditLog, error) {
	query := fmt.Sprintf(
		`SELECT %s, %s, %s, %s, %s FROM %s ORDER BY 1`,
		sqliteQuoteIdent(plugin.DriverAuditColumnID.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnData.Name),
		sqliteQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
		sqliteTableName(execCtx, plugin.DriverAuditLogTableName),
	)
	rows, err := c.Read.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	logs := make([]plugin.DriverAuditLog, 0)
	for rows.Next() {
		var entry plugin.DriverAuditLog
		var appliedAt string
		var data string
		var metadata sql.NullString

		if err := rows.Scan(&entry.ID, &appliedAt, &entry.Event, &data, &metadata); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		entry.AppliedAt, err = time.Parse(time.RFC3339Nano, appliedAt)
		if err != nil {
			return nil, fmt.Errorf("parse time: %w", err)
		}

		if err := json.Unmarshal([]byte(data), &entry.Data); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}

		if metadata.Valid {
			if err := json.Unmarshal([]byte(metadata.String), &entry.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		logs = append(logs, entry)
	}
	return logs, nil
}

func (c *SQLiteDriverConnection) LockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	data, err := json.Marshal(lock.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?)`,
		sqliteTableName(execCtx, plugin.DriverAuditLockTableName),
		sqliteQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		sqliteQuoteIdent(plugin.DriverAuditLockColumnLockedAt.Name),
		sqliteQuoteIdent(plugin.DriverAuditLockColumnLockedBy.Name),
		sqliteQuoteIdent(plugin.DriverAuditLockColumnData.Name),
	)
	_, err = c.Write.ExecContext(ctx, query,
		lock.ID,
		lock.LockedAt.Format(time.RFC3339Nano),
		lock.LockedBy,
		string(data),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return plugin.ErrAuditAlreadyLocked
		}
		return err
	}
	return nil
}

func (c *SQLiteDriverConnection) UnlockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	_, err := c.Write.ExecContext(ctx,
		fmt.Sprintf(
			`DELETE FROM %s WHERE %s = ?`,
			sqliteTableName(execCtx, plugin.DriverAuditLockTableName),
			sqliteQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		),
		lock.ID,
	)
	return err
}

func (c *SQLiteDriverConnection) Query(execCtx plugin.DriverExecutionContext) plugin.DriverQuery {
	return &SQLiteDriverQuery{conn: c}
}

func sqliteQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sqliteTableName(execCtx plugin.DriverExecutionContext, table string) string {
	// SQLite has no schema support; ignore execCtx.Schema
	return sqliteQuoteIdent(execCtx.Prefix + table)
}

func sqliteColumnDefinition(column *plugin.DriverAuditColumn) string {
	colType := "UNKNOWN"
	switch column.Type {
	case plugin.DriverAuditColumnTime:
		colType = "TEXT"
	case plugin.DriverAuditColumnString:
		colType = "TEXT"
	case plugin.DriverAuditColumnInt64:
		colType = "INTEGER"
	case plugin.DriverAuditColumnJSON:
		colType = "TEXT"
	}

	constraint := ""
	if column.PrimaryKey {
		constraint = " PRIMARY KEY"
	} else if !column.Nullable {
		constraint = " NOT NULL"
	}

	return fmt.Sprintf("%s %s%s", sqliteQuoteIdent(column.Name), colType, constraint)
}

type SQLiteDriverQuery struct {
	conn *SQLiteDriverConnection
	tx   *sql.Tx
}

func (q *SQLiteDriverQuery) Exec(ctx context.Context, query string, args ...any) error {
	if q.tx != nil {
		_, err := q.tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
		return nil
	}
	_, err := q.conn.Write.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	return nil
}

func (q *SQLiteDriverQuery) Query(ctx context.Context, query string, args ...any) (*plugin.DriverQueryResult, error) {
	var rows *sql.Rows
	var err error
	if q.tx != nil {
		rows, err = q.tx.QueryContext(ctx, query, args...)
	} else {
		rows, err = q.conn.Read.QueryContext(ctx, query, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	result := &plugin.DriverQueryResult{
		AffectedRows: 0,
		Rows:         make([][]any, 0),
	}
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to row scan: %w", err)
		}
		result.Rows = append(result.Rows, values)
        result.AffectedRows += 1
	}

	return result, nil
}

func (q *SQLiteDriverQuery) Begin(ctx context.Context) (plugin.DriverQuery, error) {
	tx, err := q.conn.Write.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &SQLiteDriverQuery{conn: q.conn, tx: tx}, nil
}

func (q *SQLiteDriverQuery) Commit(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to commit")
	}
	if err := q.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	q.tx = nil
	return nil
}

func (q *SQLiteDriverQuery) Rollback(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to rollback")
	}
	if err := q.tx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	q.tx = nil
	return nil
}
