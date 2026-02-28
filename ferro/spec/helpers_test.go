package spec

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/qbart/ferrodb/ferro"
	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
	"github.com/qbart/ferrodb/ferro/run"
	"github.com/qbart/ferrodb/fmtx"
	"github.com/qbart/ferrodb/plugins"
	"github.com/qbart/ferrodb/tpls"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/wzshiming/ctc"
)

type cliMock struct {
	connection *mockDBConnection
	app        *ferro.App
	exitCode   int
	fs         afero.Afero
	id         string
	stdout     *bytes.Buffer
	stderr     *bytes.Buffer
	T          *testing.T
	*clockMock
	//
	templates  *tpls.Templates
	registry   *plugins.Registry
	filesystem *config.Filesystem
}

type clockMock struct {
	Time time.Time
}

type assertAudit struct {
	result *run.MigrateAuditResult
	T      *testing.T
}

type assertData struct {
	T              *testing.T
	driverInstance *plugin.DriverInstance
	set            *config.MigrationSet
	nav            *run.Navigator
	execCtx        plugin.DriverExecutionContext
}

func NewTestCLI(t *testing.T) (*cliMock, func()) {
	dir := t.TempDir()
	osfs := afero.NewOsFs()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	clock := &clockMock{
		Time: time.Now().UTC(),
	}
	app := &ferro.App{
		Logger: fmtx.New(stdout, stderr),
		Dir:    dir,
		Clock:  clock,
	}

	templates := tpls.New(template.FuncMap{})
	registry := plugins.New()
	registry.RegisterAll()
	filesystem := config.NewFilesystem(dir)

	teardown := func() {
		// tempdir automatically cleans after finished test
	}

	return &cliMock{
		T:          t,
		id:         uuid.Must(uuid.NewV7()).String(),
		fs:         afero.Afero{Fs: osfs},
		app:        app,
		stdout:     stdout,
		stderr:     stderr,
		templates:  templates,
		registry:   registry,
		filesystem: filesystem,
		clockMock:  clock,
	}, teardown
}

func (c *cliMock) SetTime(stime string) {
	loc := time.Now().Location()
	layout := "2006-01-02 15:04"
	t, err := time.ParseInLocation(layout, stime, loc)
	if err != nil {
		c.T.Fatalf("failed to parse time: %v", err)
	}
	c.clockMock.Time = t.UTC()
}

func (c *cliMock) RandomDatabase() func() {
	testPluginDriver := os.Getenv("TEST_DRIVER")
	switch testPluginDriver {
	case "postgresql":
		return c.RandomPostreSQLDatabase()
	case "sqlite":
		return c.RandomSQLiteDatabase()
	default:
		panic(fmt.Errorf("unhandled test driver: %s", testPluginDriver))
	}
}

func (c *cliMock) RandomSQLiteDatabase() func() {
	dbID := fmt.Sprintf("test_%s.sqlite", strings.ReplaceAll(uuid.NewString(), "-", ""))
	os.MkdirAll("tmp", 0755)
	path := filepath.Join("tmp", dbID)
	// execCtx := plugin.DriverExecutionContext{} no needed for sqlite at creation
	ctx := context.Background()

	driver := plugins.NewSQLiteDriver()
	err := driver.CreateDatabase(ctx, path)
	if err != nil {
		c.T.Fatalf("failed to create database %v", err)
	}

	dbTeardown := func() {
		err := os.Remove(path)
		if err != nil {
			c.T.Fatalf("failed to remove database %v", err)
		}
	}
	c.Files(
		"config.fyml",
		fmt.Sprintf(`
apiVersion: drivers/v1
kind: Driver
metadata:
  name: test
spec:
  driver: sqlite
  config:
    path: %s
        `, path),
	)

	return dbTeardown
}

func (c *cliMock) RandomPostreSQLDatabase() func() {
	dbID := fmt.Sprintf("test_%s", strings.ReplaceAll(uuid.NewString(), "-", ""))
	execCtx := plugin.DriverExecutionContext{
		Schema: "public",
	}
	ctx := context.Background()

	driver := plugins.NewPostgreSQLDriver()
	conn, err := driver.Connect(context.Background(), config.DriverConfig{
		"dsn": "postgres://test:test@localhost:5433/test",
	})
	if err != nil {
		c.T.Fatalf("failed to connect to test database: %v", err)
	}
	defer driver.Disconnect(ctx, conn)

	// create database and grant privileges for the test case
	err = conn.Query(execCtx).Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbID))
	if err != nil {
		c.T.Fatalf("failed to create database: %v", err)
	}
	err = conn.Query(execCtx).Exec(ctx, fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO test;", dbID))
	if err != nil {
		c.T.Fatalf("failed to grant access to database %v", err)
	}

	dbTeardown := func() {
		conn, err := driver.Connect(ctx, config.DriverConfig{
			"dsn": "postgres://test:test@localhost:5433/test",
		})
		if err != nil {
			c.T.Fatalf("failed to connect to test database to perform cleanup: %v", err)
		}
		defer driver.Disconnect(ctx, conn)

		// cleanup
		err = conn.Query(execCtx).Exec(ctx, fmt.Sprintf("DROP DATABASE %s", dbID))
		if err != nil {
			c.T.Fatalf("failed to drop database %v", err)
		}
	}
	c.Files(
		"config.fyml",
		fmt.Sprintf(`
apiVersion: drivers/v1
kind: Driver
metadata:
  name: test
spec:
  driver: postgresql
  config:
    dsn: postgres://test:test@localhost:5433/%s
        `, dbID),
	)

	return dbTeardown
}

func (c *cliMock) DefaultDatabase() {
	c.Files(
		"config.fyml",
		`
apiVersion: drivers/v1
kind: Driver
metadata:
  name: test
spec:
  driver: postgresql
  config:
    dsn: postgres://test:test@localhost:5433/test
        `,
	)
}

func (c *cliMock) Checksum(migration string) string {
	config, err := c.Runner().UseConfig()
	if err != nil {
		c.T.Fatalf("cannot use config for checksum: %v", err)
	}
	mig, ok := config.Migrations[migration]
	if !ok {
		c.T.Fatalf("migration `%s` does not exist to calculate checkum", migration)
	}
	return string(mig.Checksum)
}

func (c *cliMock) Files(pathContentPair ...string) {
	for i := 1; i < len(pathContentPair); i += 2 {
		path := filepath.Join(c.app.Dir, pathContentPair[i-1])
		content := pathContentPair[i]
		afero.WriteFile(
			c.fs,
			path,
			[]byte(content),
			0644,
		)
	}
}

func (m *cliMock) AssertRun(args ...string) bool {
	m.setup(args)
	m.exitCode = m.app.Run(append([]string{"ferro"}, args...))

	if assert.Equal(m.T, 0, m.exitCode, "Exit code should be eql to 0") {
		return true
	} else {
		fmt.Println("statements debug:")
		for _, sql := range m.connection.recorder {
			fmt.Println("---")
			fmt.Println(ctc.ForegroundBrightRed, sql, ctc.Reset)
		}
		fmt.Println("---")
		fmt.Println(ctc.ForegroundRed, m.stderr.String(), ctc.Reset)
		fmt.Println(ctc.ForegroundRed, m.stdout.String(), ctc.Reset)
		return false
	}
}

func (m *cliMock) AssertSchemaMigrationTable(t *testing.T, schema string, expectedVersions ...string) bool {
	panic("AssertSchemaMigrationTable not implemented")
	return true
}

func (m *cliMock) setup(args []string) {
	m.connection = &mockDBConnection{
		recorder:         []string{},
		assertedSQLIndex: 0,
	}
}

func (m *cliMock) AssertNotRun(args ...string) bool {
	m.setup(args)
	m.exitCode = m.app.Run(append([]string{"ferro"}, args...))

	return assert.Greater(m.T, m.exitCode, 0, "Exit code should be greather than 0")
}

func (m *cliMock) AssertOutputContains(output string) bool {
	s := fmtx.StripANSI(m.stdout.String())
	chunks := strings.Split(s, "\n")
	for i, chunk := range chunks {
		chunks[i] = fmtx.Squish(chunk)
	}
	s = strings.Join(chunks, "\n")

	chunks = strings.Split(output, "\n")
	for i, chunk := range chunks {
		chunks[i] = fmtx.Squish(chunk)
	}
	output = strings.Join(chunks, "\n")

	val := assert.Contains(
		m.T,
		strings.TrimSpace(s),
		strings.TrimSpace(output),
		"Output mismatch",
	)
	if !val {
		m.T.Logf("Expected:\n%s", output)
		m.T.Fatalf("Captured:\n%s", m.stdout.String())
	}
	return val
}

func (m *cliMock) AssertErrorContains(output string) bool {
	s := fmtx.StripANSI(m.stderr.String())
	chunks := strings.Split(s, "\n")
	for i, chunk := range chunks {
		chunks[i] = fmtx.Squish(chunk)
	}
	s = strings.Join(chunks, "\n")

	chunks = strings.Split(output, "\n")
	for i, chunk := range chunks {
		chunks[i] = fmtx.Squish(chunk)
	}
	output = strings.Join(chunks, "\n")

	val := assert.Contains(
		m.T,
		strings.TrimSpace(s),
		strings.TrimSpace(output),
		"Output mismatch",
	)
	if !val {
		m.T.Fatalf("Captured:\n%s", m.stderr.String())
	}
	return val
}

func (m *cliMock) AssertOutputNotContains(output string) bool {
	s := fmtx.StripANSI(m.stdout.String())
	chunks := strings.Split(s, "\n")
	for i, chunk := range chunks {
		chunks[i] = fmtx.Squish(chunk)
	}
	s = strings.Join(chunks, "\n")

	chunks = strings.Split(output, "\n")
	for i, chunk := range chunks {
		chunks[i] = fmtx.Squish(chunk)
	}
	output = strings.Join(chunks, "\n")

	val := assert.NotContains(
		m.T,
		strings.TrimSpace(s),
		strings.TrimSpace(output),
		"Output mismatch",
	)
	if !val {
		m.T.Fatalf("Captured:\n%s", m.stdout.String())
	}
	return val
}

func (m *cliMock) ResetAllOutputs() {
	m.stdout.Reset()
	m.stderr.Reset()
}

func (m *cliMock) ResetDriverOutputs() {
	m.connection.assertedSQLIndex = 0
	m.connection.recorder = []string{}
}

func (m *cliMock) Runner() *run.Runner {
	templates := tpls.New(template.FuncMap{})
	registry := plugins.New()
	registry.RegisterAll()
	filesystem := config.NewFilesystem(m.app.Dir)
	return run.New(filesystem, templates, registry, m.app.Logger, m.clockMock)
}

func (m *cliMock) Audit(driver string, set string) *assertAudit {
	result, err := m.Runner().ExecuteMigrateAudit(context.Background(), &run.CommandAudit{
		Driver:   driver,
		Set:      set,
		N:        0,
		FullView: false,
	})
	if err != nil {
		m.T.Fatalf("can read audit: %v", err)
	}
	return &assertAudit{
		T:      m.T,
		result: result,
	}
}

func (a *assertAudit) AssertCount(count int) {
	if count != len(a.result.Logs) {
		// panic("AssertTableExists not implemented")
		a.T.Fatalf("expect to have %d audit logs, but got %d", count, len(a.result.Logs))
	}
}

type auditLog struct {
	ID       int64
	Event    string
	Data     map[string]any
	Metadata map[string]any
}

func (a *assertAudit) Assert(index int, log auditLog) {
	if index >= len(a.result.Logs) {
		a.T.Fatalf("no enough logs in audit")
	}

	got := a.result.Logs[index]
	want := plugin.DriverAuditLog{
		ID:        log.ID,
		AppliedAt: got.AppliedAt,
		Event:     log.Event,
		Data:      log.Data,
		Metadata:  log.Metadata,
	}

	// // we need to compare it individual keys for partial match
	// gotM := got.Metadata
	// wantM := log.Metadata

    // want.Metadata = map[string]any{}
	// got.Metadata = map[string]any{}

	if !reflect.DeepEqual(got, want) {
		a.T.Logf("audit log[%d] is not same", index)
		a.T.Logf("   got:%v", got)
		a.T.Logf("  want:%v", want)
		a.T.FailNow()
	}

	// if len(gotM) != len(wantM) {
	// 	a.T.Logf("audit log[%d] metadata is not same", index)
	// 	a.T.Logf("   got:%v", gotM)
	// 	a.T.Logf("  want:%v", wantM)
	// 	a.T.FailNow()
	// }
	//
	// for k := range wantM {
	// 	fail := false
	// 	switch wantM[k].(type) {
	// 	case PartialStringComparator:
	// 		a := gotM[k].(string)
	// 		b := wantM[k].(PartialStringComparator).S
	// 		fail = strings.Index(a, b) == -1
	//
	// 	default:
	// 		fail = !assert.Equal(a.T, gotM[k], wantM[k])
	// 	}
	//
	// 	if fail {
	// 		a.T.Logf("Invalid metadata")
	// 		a.T.FailNow()
	// 	}
	// }
}

func (m *cliMock) Data(driver string, set string) *assertData {
	runner := m.Runner()
	config, err := runner.UseConfig()
	if err != nil {
		m.T.Fatalf("cannot use config, runner error: %v", err)
	}
	instance, err := runner.UseDriver(config, driver)
	if err != nil {
		m.T.Fatalf("cannot use driver, runner error: %v", err)
	}
	migrationSet, err := runner.UseMigrationSet(config, set)
	if err != nil {
		m.T.Fatalf("cannot use migration set, runner error: %v", err)
	}

	execCtx := plugin.DriverExecutionContext{
		Prefix: migrationSet.Spec.Namespace.Prefix,
		Schema: migrationSet.Spec.Namespace.Schema,
	}
	nav := run.NewNavigator(instance, config, execCtx, m.clockMock)

	return &assertData{
		T:       m.T,
		nav:     nav,
		execCtx: execCtx,
	}
}

func (d *assertData) AssertTableExists(name string) {
	conn, close, err := d.nav.Open(context.Background())
	if err != nil {
		d.T.Fatalf("failed to open navigator: %v", err)
	}
	defer close()

	result, err := tableExistsQuery(context.Background(), conn, d.execCtx, name)
	if err != nil {
		d.T.Fatalf("failed to check if table exists: %v", err)
	}
	if result.AffectedRows != 1 {
		d.T.Fatalf("query returned invalid number of rows(%d)", result.AffectedRows)
	}

	exists := result.Rows[0][0].(int64)
	if exists == 0 {
		d.T.Fatalf("table `%s` does not exist but it should", name)
	}
}

func (d *assertData) AssertTableNotExists(name string) {
	conn, close, err := d.nav.Open(context.Background())
	if err != nil {
		d.T.Fatalf("failed to open navigator: %v", err)
	}
	defer close()

	result, err := tableExistsQuery(context.Background(), conn, d.execCtx, name)
	if err != nil {
		d.T.Fatalf("failed to check if table exists: %v", err)
	}
	if result.AffectedRows != 1 {
		d.T.Fatalf("query returned invalid number of rows(%d)", result.AffectedRows)
	}

	exists := result.Rows[0][0].(int64)
	if exists == 1 {
		d.T.Fatalf("table `%s` exists but it should not", name)
	}
}

func tableExistsQuery(ctx context.Context, conn plugin.DriverConnection, execCtx plugin.DriverExecutionContext, name string) (*plugin.DriverQueryResult, error) {
	testPluginDriver := os.Getenv("TEST_DRIVER")

	switch testPluginDriver {
	case "postgresql":
		schema := execCtx.Schema
		if schema == "" {
			schema = "public"
		}
		q := `
    SELECT CASE WHEN EXISTS (
      SELECT 1
      FROM information_schema.tables
      WHERE table_schema = $1
        AND table_name = $2
        ) THEN 1::bigint ELSE 0::bigint END;`
		return conn.Query(execCtx).Query(ctx, q, schema, execCtx.Prefix+name)

	case "sqlite":
		q := `
    SELECT EXISTS (
      SELECT 1
      FROM sqlite_schema
      WHERE type = 'table'
        AND name = ?
        AND name NOT LIKE 'sqlite_%'
    );`
		schema := execCtx.Schema
		if schema == "" {
			schema = "main"
		}
		return conn.Query(execCtx).Query(context.Background(), q, execCtx.Prefix+name)

	default:
		panic(fmt.Errorf("unhandled test driver: %s", testPluginDriver))
	}
}

func migrationError() string {
	testPluginDriver := os.Getenv("TEST_DRIVER")

	switch testPluginDriver {
	case "postgresql":
        return `ERROR: syntax error at or near ";" (SQLSTATE 42601)`

	case "sqlite":
        return `SQL logic error: near ";": syntax error (1)`

	default:
		panic(fmt.Errorf("unhandled test driver: %s", testPluginDriver))
	}
}

func (c *clockMock) Now() time.Time {
	return c.Time
}

type PartialStringComparator struct {
	S string
}

func stringLike(s string) PartialStringComparator {
	return PartialStringComparator{
		S: s,
	}
}
