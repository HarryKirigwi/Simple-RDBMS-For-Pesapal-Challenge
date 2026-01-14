package storage

import (
	"rdbms/internal/types"
)

// Row represents a row of data
type Row struct {
	Values []*types.Value
}

// StorageEngine is the interface for storage operations
type StorageEngine interface {
	CreateTable(schema *TableSchema) error
	Insert(tableName string, row *Row) error
	Select(tableName string, filter func(*Row) bool) ([]*Row, error)
	Update(tableName string, filter func(*Row) bool, update func(*Row) *Row) (int, error)
	Delete(tableName string, filter func(*Row) bool) (int, error)
	GetSchema(tableName string) (*TableSchema, error)
	ListTables() ([]string, error)
	Close() error
}
