package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type ClickHouseDriver struct{}

type ClickHouseDriverConnection struct {
	DB *sql.DB
}

func NewClickHouseDriver() *ClickHouseDriver {
	return &ClickHouseDriver{}
}

func (d *ClickHouseDriver) Connect(ctx context.Context, config config.DriverConfig) (plugin.DriverConnection, error) {
	dsn := config.String("dsn")
	if dsn == "" {
		return nil, fmt.Errorf("config.dsn is required")
	}

	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid ClickHouse DSN: %w", err)
	}

	db := clickhouse.OpenDB(opts)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	return &ClickHouseDriverConnection{DB: db}, nil
}

func (d *ClickHouseDriver) Disconnect(ctx context.Context, conn plugin.DriverConnection) error {
	driverConn := conn.(*ClickHouseDriverConnection)
	if err := driverConn.DB.Close(); err != nil {
		return fmt.Errorf("failed to disconnect from ClickHouse: %w", err)
	}
	return nil
}

func (d *ClickHouseDriver) IsNamespaceSupported() bool { return false }

func (c *ClickHouseDriverConnection) UpsertAuditLogTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		chColumnDefinition(&plugin.DriverAuditColumnID),
		chColumnDefinition(&plugin.DriverAuditColumnAppliedAt),
		chColumnDefinition(&plugin.DriverAuditColumnEvent),
		chColumnDefinition(&plugin.DriverAuditColumnData),
		chColumnDefinition(&plugin.DriverAuditColumnMetadata),
	}
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s) ENGINE = MergeTree() ORDER BY %s`,
			chTableName(execCtx, plugin.DriverAuditLogTableName),
			strings.Join(columns, ","),
			chQuoteIdent(plugin.DriverAuditColumnID.Name),
		),
	)
	return err
}

func (c *ClickHouseDriverConnection) UpsertAuditLockTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		chColumnDefinition(&plugin.DriverAuditLockColumnID),
		chColumnDefinition(&plugin.DriverAuditLockColumnLockedAt),
		chColumnDefinition(&plugin.DriverAuditLockColumnLockedBy),
		chColumnDefinition(&plugin.DriverAuditLockColumnData),
	}
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s) ENGINE = MergeTree() ORDER BY %s`,
			chTableName(execCtx, plugin.DriverAuditLockTableName),
			strings.Join(columns, ","),
			chQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		),
	)
	return err
}

func (c *ClickHouseDriverConnection) AppendAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, log plugin.DriverAuditLog) error {
	data, err := json.Marshal(log.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	var metadataStr string
	if log.Metadata != nil {
		b, err := json.Marshal(log.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metadataStr = string(b)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?)`,
		chTableName(execCtx, plugin.DriverAuditLogTableName),
		chQuoteIdent(plugin.DriverAuditColumnID.Name),
		chQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		chQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		chQuoteIdent(plugin.DriverAuditColumnData.Name),
		chQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
	)
	_, err = c.DB.ExecContext(ctx, query,
		log.ID,
		log.AppliedAt.UTC(),
		log.Event,
		string(data),
		metadataStr,
	)
	return err
}

func (c *ClickHouseDriverConnection) ReadAuditLogs(ctx context.Context, execCtx plugin.DriverExecutionContext) ([]plugin.DriverAuditLog, error) {
	query := fmt.Sprintf(
		`SELECT %s, %s, %s, %s, %s FROM %s ORDER BY 1`,
		chQuoteIdent(plugin.DriverAuditColumnID.Name),
		chQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		chQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		chQuoteIdent(plugin.DriverAuditColumnData.Name),
		chQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
		chTableName(execCtx, plugin.DriverAuditLogTableName),
	)
	rows, err := c.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	logs := make([]plugin.DriverAuditLog, 0)
	for rows.Next() {
		var entry plugin.DriverAuditLog
		var data string
		var metadata string

		if err := rows.Scan(&entry.ID, &entry.AppliedAt, &entry.Event, &data, &metadata); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		if err := json.Unmarshal([]byte(data), &entry.Data); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}

		if metadata != "" {
			if err := json.Unmarshal([]byte(metadata), &entry.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		logs = append(logs, entry)
	}
	return logs, nil
}

func (c *ClickHouseDriverConnection) LockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	// Check if lock exists
	var count uint64
	err := c.DB.QueryRowContext(ctx,
		fmt.Sprintf(
			`SELECT count() FROM %s WHERE %s = ?`,
			chTableName(execCtx, plugin.DriverAuditLockTableName),
			chQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		),
		lock.ID,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return plugin.ErrAuditAlreadyLocked
	}

	data, err := json.Marshal(lock.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?)`,
		chTableName(execCtx, plugin.DriverAuditLockTableName),
		chQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		chQuoteIdent(plugin.DriverAuditLockColumnLockedAt.Name),
		chQuoteIdent(plugin.DriverAuditLockColumnLockedBy.Name),
		chQuoteIdent(plugin.DriverAuditLockColumnData.Name),
	)
	_, err = c.DB.ExecContext(ctx, query,
		lock.ID,
		lock.LockedAt.UTC(),
		lock.LockedBy,
		string(data),
	)
	return err
}

func (c *ClickHouseDriverConnection) UnlockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`ALTER TABLE %s DELETE WHERE %s = ?`,
			chTableName(execCtx, plugin.DriverAuditLockTableName),
			chQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		),
		lock.ID,
	)
	return err
}

func (c *ClickHouseDriverConnection) Query(execCtx plugin.DriverExecutionContext) plugin.DriverQuery {
	return &ClickHouseQuery{conn: c}
}

func chQuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func chTableName(execCtx plugin.DriverExecutionContext, table string) string {
	fullName := chQuoteIdent(execCtx.Prefix + table)
	if execCtx.Schema != "" {
		return chQuoteIdent(execCtx.Schema) + "." + fullName
	}
	return fullName
}

func chColumnDefinition(column *plugin.DriverAuditColumn) string {
	colType := "String"
	switch column.Type {
	case plugin.DriverAuditColumnTime:
		colType = "DateTime64(6, 'UTC')"
	case plugin.DriverAuditColumnString:
		colType = "String"
	case plugin.DriverAuditColumnInt64:
		colType = "Int64"
	case plugin.DriverAuditColumnJSON:
		colType = "String"
	}

	nullable := ""
	if column.Nullable {
		colType = "Nullable(" + colType + ")"
	} else if !column.PrimaryKey {
		nullable = ""
	}
	_ = nullable

	return fmt.Sprintf("%s %s", chQuoteIdent(column.Name), colType)
}

type ClickHouseQuery struct {
	conn *ClickHouseDriverConnection
	tx   *sql.Tx
}

func (q *ClickHouseQuery) Exec(ctx context.Context, query string, args ...any) error {
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

func (q *ClickHouseQuery) Query(ctx context.Context, query string, args ...any) (*plugin.DriverQueryResult, error) {
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

func (q *ClickHouseQuery) Begin(ctx context.Context) (plugin.DriverQuery, error) {
	tx, err := q.conn.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &ClickHouseQuery{conn: q.conn, tx: tx}, nil
}

func (q *ClickHouseQuery) Commit(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to commit")
	}
	if err := q.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	q.tx = nil
	return nil
}

func (q *ClickHouseQuery) Rollback(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to rollback")
	}
	if err := q.tx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	q.tx = nil
	return nil
}
