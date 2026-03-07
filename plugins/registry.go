package plugins

import (
	"fmt"

	"github.com/qbart/ferrodb/ferro/plugin"
	"github.com/qbart/ferrodb/plugins/testcontainers"
)

type Registry struct {
	drivers  map[string]plugin.Driver
	browsers map[string]plugin.Browser
}

func New() *Registry {
	return &Registry{
		drivers:  make(map[string]plugin.Driver),
		browsers: make(map[string]plugin.Browser),
	}
}

func (r *Registry) Register(name string, driver plugin.Driver) {
	if _, ok := r.drivers[name]; ok {
		panic("driver already registered: " + name)
	}
	r.drivers[name] = driver
}

func (r *Registry) Get(name string) (plugin.Driver, error) {
	driver, ok := r.drivers[name]
	if !ok {
		return nil, fmt.Errorf("driver not found: %s", name)
	}
	return driver, nil
}

func (r *Registry) RegisterBrowser(name string, browser plugin.Browser) {
	if _, ok := r.browsers[name]; ok {
		panic("browser already registered: " + name)
	}
	r.browsers[name] = browser
}

func (r *Registry) GetBrowser(name string) (plugin.Browser, error) {
	browser, ok := r.browsers[name]
	if !ok {
		return nil, fmt.Errorf("browser not found: %s", name)
	}
	return browser, nil
}

func (r *Registry) RegisterAll() {
	r.Register("null", NewNullDriver())
	r.Register("testcontainer/postgresql", testcontainers.NewTestContainerPostgreSQLDriver())
	r.Register("postgresql", NewPostgreSQLDriver())
	r.Register("sqlite", NewSQLiteDriver())
	r.Register("mysql", NewMySQLDriver())
	r.Register("mariadb", NewMySQLDriver())

	r.RegisterBrowser("postgresql", NewPostgreSQLBrowser(NewPostgreSQLDriver()))
	r.RegisterBrowser("sqlite", NewSQLiteBrowser(NewSQLiteDriver()))
	r.RegisterBrowser("mysql", NewMySQLBrowser(NewMySQLDriver()))
	r.RegisterBrowser("mariadb", NewMySQLBrowser(NewMySQLDriver()))
}
