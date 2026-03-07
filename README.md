# FerroDB

FerroDB is a database management tool. Focused on explicit, auditable schema migrations with built-in terminal browser.

Currently supported databases: PostgreSQL, MySQL, SQLite, Clickhouse (standalone server, table engine = MergeTree).

<img src="./res/ferrodb.svg" width="64" />

[![Go Report Card](https://goreportcard.com/badge/github.com/qbart/krab)](https://goreportcard.com/report/github.com/qbart/ferrodb)
[![Last commit](https://img.shields.io/github/last-commit/qbart/ferrodb)](https://github.com/qbart/ferrodb/commits/master)
![CI](https://github.com/qbart/ferrodb/actions/workflows/ci.yml/badge.svg)

## Roadmap

- [ ] checksum handling and operation
- [ ] drivers (in progress)
- [ ] manual audit fixing
- [ ] documentation

## On hold

Drivers:

- [ ] DuckDB (needs CGO, will be done later)
- [ ] MSSQL (no unless high demand)
- [ ] Oracle (no unless high demand)
- [ ] Clickhouse clustered (need more info from community what is expected)

## Contributing guide

Ask before.
