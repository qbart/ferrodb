package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type MySQLDriver struct{}

type MySQLDriverConnection struct {
	DB *sql.DB
}

func NewMySQLDriver() *MySQLDriver {
	return &MySQLDriver{}
}

func (d *MySQLDriver) Connect(ctx context.Context, config config.DriverConfig) (plugin.DriverConnection, error) {
	dsn := config.String("dsn")
	if dsn == "" {
		return nil, fmt.Errorf("config.dsn is required")
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid MySQL DSN: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	dsn = cfg.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping MySQL database: %w", err)
	}

	return &MySQLDriverConnection{DB: db}, nil
}

func (d *MySQLDriver) Disconnect(ctx context.Context, conn plugin.DriverConnection) error {
	driverConn := conn.(*MySQLDriverConnection)
	if err := driverConn.DB.Close(); err != nil {
		return fmt.Errorf("failed to disconnect from MySQL database: %w", err)
	}
	return nil
}

func (c *MySQLDriverConnection) UpsertAuditLogTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		mysqlColumnDefinition(&plugin.DriverAuditColumnID),
		mysqlColumnDefinition(&plugin.DriverAuditColumnAppliedAt),
		mysqlColumnDefinition(&plugin.DriverAuditColumnEvent),
		mysqlColumnDefinition(&plugin.DriverAuditColumnData),
		mysqlColumnDefinition(&plugin.DriverAuditColumnMetadata),
	}
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s)`,
			mysqlTableName(execCtx, plugin.DriverAuditLogTableName),
			strings.Join(columns, ","),
		),
	)
	return err
}

func (c *MySQLDriverConnection) UpsertAuditLockTable(ctx context.Context, execCtx plugin.DriverExecutionContext) error {
	columns := []string{
		mysqlColumnDefinition(&plugin.DriverAuditLockColumnID),
		mysqlColumnDefinition(&plugin.DriverAuditLockColumnLockedAt),
		mysqlColumnDefinition(&plugin.DriverAuditLockColumnLockedBy),
		mysqlColumnDefinition(&plugin.DriverAuditLockColumnData),
	}
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (%s)`,
			mysqlTableName(execCtx, plugin.DriverAuditLockTableName),
			strings.Join(columns, ","),
		),
	)
	return err
}

func (c *MySQLDriverConnection) AppendAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, log plugin.DriverAuditLog) error {
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
		mysqlTableName(execCtx, plugin.DriverAuditLogTableName),
		mysqlQuoteIdent(plugin.DriverAuditColumnID.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnData.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
	)
	_, err = c.DB.ExecContext(ctx, query,
		log.ID,
		log.AppliedAt.UTC(),
		log.Event,
		string(data),
		metadataArg,
	)
	return err
}

func (c *MySQLDriverConnection) ReadAuditLogs(ctx context.Context, execCtx plugin.DriverExecutionContext) ([]plugin.DriverAuditLog, error) {
	query := fmt.Sprintf(
		`SELECT %s, %s, %s, %s, %s FROM %s ORDER BY 1`,
		mysqlQuoteIdent(plugin.DriverAuditColumnID.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnAppliedAt.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnEvent.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnData.Name),
		mysqlQuoteIdent(plugin.DriverAuditColumnMetadata.Name),
		mysqlTableName(execCtx, plugin.DriverAuditLogTableName),
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
		var metadata sql.NullString

		if err := rows.Scan(&entry.ID, &entry.AppliedAt, &entry.Event, &data, &metadata); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
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

func (c *MySQLDriverConnection) LockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	data, err := json.Marshal(lock.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?)`,
		mysqlTableName(execCtx, plugin.DriverAuditLockTableName),
		mysqlQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		mysqlQuoteIdent(plugin.DriverAuditLockColumnLockedAt.Name),
		mysqlQuoteIdent(plugin.DriverAuditLockColumnLockedBy.Name),
		mysqlQuoteIdent(plugin.DriverAuditLockColumnData.Name),
	)
	_, err = c.DB.ExecContext(ctx, query,
		lock.ID,
		lock.LockedAt.UTC(),
		lock.LockedBy,
		string(data),
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return plugin.ErrAuditAlreadyLocked
		}
		return err
	}
	return nil
}

func (c *MySQLDriverConnection) UnlockAuditLog(ctx context.Context, execCtx plugin.DriverExecutionContext, lock plugin.DriverAuditLock) error {
	_, err := c.DB.ExecContext(ctx,
		fmt.Sprintf(
			`DELETE FROM %s WHERE %s = ?`,
			mysqlTableName(execCtx, plugin.DriverAuditLockTableName),
			mysqlQuoteIdent(plugin.DriverAuditLockColumnID.Name),
		),
		lock.ID,
	)
	return err
}

func (c *MySQLDriverConnection) Query(execCtx plugin.DriverExecutionContext) plugin.DriverQuery {
	return &MySQLQuery{conn: c, tx: nil}
}

func mysqlQuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func mysqlTableName(execCtx plugin.DriverExecutionContext, table string) string {
	fullName := mysqlQuoteIdent(execCtx.Prefix + table)
	if execCtx.Schema != "" {
		return mysqlQuoteIdent(execCtx.Schema) + "." + fullName
	}
	return fullName
}

func mysqlColumnDefinition(column *plugin.DriverAuditColumn) string {
	colType := "UNKNOWN"
	switch column.Type {
	case plugin.DriverAuditColumnTime:
		colType = "DATETIME(6)"
	case plugin.DriverAuditColumnString:
		colType = "VARCHAR(255)"
	case plugin.DriverAuditColumnInt64:
		colType = "BIGINT"
	case plugin.DriverAuditColumnJSON:
		colType = "JSON"
	}

	constraint := ""
	if column.PrimaryKey {
		constraint = " PRIMARY KEY"
	} else if !column.Nullable {
		constraint = " NOT NULL"
	}

	return fmt.Sprintf("%s %s%s", mysqlQuoteIdent(column.Name), colType, constraint)
}

type MySQLQuery struct {
	conn *MySQLDriverConnection
	tx   *sql.Tx
}

func (q *MySQLQuery) Exec(ctx context.Context, query string, args ...any) error {
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

func (q *MySQLQuery) Query(ctx context.Context, query string, args ...any) (*plugin.DriverQueryResult, error) {
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

func (q *MySQLQuery) Begin(ctx context.Context) (plugin.DriverQuery, error) {
	tx, err := q.conn.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &MySQLQuery{conn: q.conn, tx: tx}, nil
}

func (q *MySQLQuery) Commit(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to commit")
	}
	if err := q.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	q.tx = nil
	return nil
}

func (q *MySQLQuery) Rollback(ctx context.Context) error {
	if q.tx == nil {
		return errors.New("no transaction to rollback")
	}
	if err := q.tx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	q.tx = nil
	return nil
}
