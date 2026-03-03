package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type DuckDBDriver struct {
	Path string
}

type DuckDBDriverConnection struct {
	DB *sql.DB
}

func NewDuckDBDriver() *DuckDBDriver {
	return &DuckDBDriver{}
}

func (d *DuckDBDriver) CreateDatabase(ctx context.Context, path string) error {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	return nil
}

func (d *DuckDBDriver) Connect(ctx context.Context, config config.DriverConfig) (plugin.DriverConnection, error) {
	path := config.String("path")

	db, err := sql.Open("duckdb", path)
	if err != nil {
		return &DuckDBDriverConnection{}, err
	}

	conn := &DuckDBDriverConnection{
		DB: db,
	}
	return conn, nil
}

func (d *DuckDBDriver) Disconnect(ctx context.Context, conn plugin.DriverConnection) error {
	driverConn := conn.(*DuckDBDriverConnection)
	if err := driverConn.DB.Close(); err != nil {
		return fmt.Errorf("failed to disconnect from DuckDB database: %w", err)
	}
	return nil
}

func (c *DuckDBDriverConnection) UpsertAuditLogTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		duckdbColumnDefinition(&plugin.DriverAuditColumnID),
		duckdbColumnDefinition(&plugin.DriverAuditColumnAppliedAt),
		duckdbColumnDefinition(&plugin.DriverAuditColumnEvent),
		duckdbColumnDefinition(&plugin.DriverAuditColumnData),
		duckdbColumnDefinition(&plugin.DriverAuditColumnMetadata),
	}
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s)`,
			duckdbTableName(execCtx, plugin.DriverAuditLogTableName),
			strings.Join(columns, ","),
		),
	)
	return err
}

func (c *DuckDBDriverConnection) UpsertAuditLockTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		duckdbColumnDefinition(&plugin.DriverAuditLockColumnID),
		duckdbColumnDefinition(&plugin.DriverAuditLockColumnLockedAt),
		duckdbColumnDefinition(&plugin.DriverAuditLockColumnLockedBy),
		duckdbColumnDefinition(&plugin.DriverAuditLockColumnData),
	}
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s)`,
			duckdbTableName(execCtx, plugin.DriverAuditLockTableName),
			strings.Join(columns, ","),
		),
	)
	return err
}

func (c *DuckDBDriverConnection) AppendAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, log plugin.DriverAuditLog) error {
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
		`INSERT INTO %s (%s, %s, %s, %s, %s) VALUES ($1, $2, $3, $4, $5)`,
		duckdbTableName(execCtx, plugin.DriverAuditLogTableName),
		duckdbQuoteIdent(plugin.DriverAuditColumnID.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnData.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
	)
	_, err = c.DB.ExecContext(ctx, query,
		log.ID,
		log.AppliedAt.UTC().Format(time.RFC3339Nano),
		log.Event,
		string(data),
		metadataArg,
	)
	return err
}

func (c *DuckDBDriverConnection) ReadAuditLogs(ctx context.Context, execCtx plugin.DriverExecutionContext) ([]plugin.DriverAuditLog, error) {
	query := fmt.Sprintf(
		`SELECT %s, %s, %s, %s, %s FROM %s ORDER BY 1`,
		duckdbQuoteIdent(plugin.DriverAuditColumnID.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnData.Name),
		duckdbQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
		duckdbTableName(execCtx, plugin.DriverAuditLogTableName),
	)
	rows, err := c.DB.QueryContext(ctx, query)
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

		entry.AppliedAt, err = time.ParseInLocation(time.RFC3339Nano, appliedAt, time.UTC)
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

func (c *DuckDBDriverConnection) LockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	data, err := json.Marshal(lock.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s) VALUES ($1, $2, $3, $4)`,
		duckdbTableName(execCtx, plugin.DriverAuditLockTableName),
		duckdbQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		duckdbQuoteIdent(plugin.DriverAuditLockColumnLockedAt.Name),
		duckdbQuoteIdent(plugin.DriverAuditLockColumnLockedBy.Name),
		duckdbQuoteIdent(plugin.DriverAuditLockColumnData.Name),
	)
	_, err = c.DB.ExecContext(ctx, query,
		lock.ID,
		lock.LockedAt.UTC().Format(time.RFC3339Nano),
		lock.LockedBy,
		string(data),
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate key") || strings.Contains(err.Error(), "PRIMARY KEY") || strings.Contains(err.Error(), "duplicate") {
			return plugin.ErrAuditAlreadyLocked
		}
		return err
	}
	return nil
}

func (c *DuckDBDriverConnection) UnlockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`DELETE FROM %s WHERE %s = $1`,
			duckdbTableName(execCtx, plugin.DriverAuditLockTableName),
			duckdbQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		),
		lock.ID,
	)
	return err
}

func (c *DuckDBDriverConnection) Query(execCtx plugin.DriverExecutionContext) plugin.DriverQuery {
	return &DuckDBDriverQuery{conn: c}
}

func duckdbQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func duckdbTableName(execCtx plugin.DriverExecutionContext, table string) string {
	fullName := duckdbQuoteIdent(execCtx.Prefix + table)
	if execCtx.Schema != "" {
		return duckdbQuoteIdent(execCtx.Schema) + "." + fullName
	}
	return fullName
}

func duckdbColumnDefinition(column *plugin.DriverAuditColumn) string {
	colType := "UNKNOWN"
	switch column.Type {
	case plugin.DriverAuditColumnTime:
		colType = "VARCHAR"
	case plugin.DriverAuditColumnString:
		colType = "VARCHAR"
	case plugin.DriverAuditColumnInt64:
		colType = "BIGINT"
	case plugin.DriverAuditColumnJSON:
		colType = "VARCHAR"
	}

	constraint := ""
	if column.PrimaryKey {
		constraint = " PRIMARY KEY"
	} else if !column.Nullable {
		constraint = " NOT NULL"
	}

	return fmt.Sprintf("%s %s%s", duckdbQuoteIdent(column.Name), colType, constraint)
}

type DuckDBDriverQuery struct {
	conn *DuckDBDriverConnection
	tx   *sql.Tx
}

func (q *DuckDBDriverQuery) Exec(ctx context.Context, query string, args ...any) error {
	if q.tx != nil {
		_, err := q.tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
		return nil
	}
	_, err := q.conn.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	return nil
}

func (q *DuckDBDriverQuery) Query(ctx context.Context, query string, args ...any) (*plugin.DriverQueryResult, error) {
	var rows *sql.Rows
	var err error
	if q.tx != nil {
		rows, err = q.tx.QueryContext(ctx, query, args...)
	} else {
		rows, err = q.conn.DB.QueryContext(ctx, query, args...)
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

func (q *DuckDBDriverQuery) Begin(ctx context.Context) (plugin.DriverQuery, error) {
	tx, err := q.conn.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &DuckDBDriverQuery{conn: q.conn, tx: tx}, nil
}

func (q *DuckDBDriverQuery) Commit(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to commit")
	}
	if err := q.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	q.tx = nil
	return nil
}

func (q *DuckDBDriverQuery) Rollback(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to rollback")
	}
	if err := q.tx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	q.tx = nil
	return nil
}
