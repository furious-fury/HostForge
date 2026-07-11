// Package database opens the control-plane SQLite file (modernc.org/sqlite, no CGO), applies
// embedded SQL migrations in lexical order, and records applied versions in schema_migrations.
package database
