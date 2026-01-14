package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rdbms/internal/types"
)

// TableSchema represents the schema of a table
type TableSchema struct {
	Name        string
	Columns     []ColumnDef
	PrimaryKey  []string
	UniqueKeys  [][]string
}

// ColumnDef represents a column in a table schema
type ColumnDef struct {
	Name     string
	Type     types.DataType
	Size     int
	Nullable bool
}

// SchemaManager manages table schemas
type SchemaManager struct {
	baseDir string
}

// NewSchemaManager creates a new schema manager
func NewSchemaManager(baseDir string) *SchemaManager {
	os.MkdirAll(baseDir, 0755)
	return &SchemaManager{baseDir: baseDir}
}

// SaveSchema saves a table schema to disk
func (sm *SchemaManager) SaveSchema(schema *TableSchema) error {
	schemaFile := filepath.Join(sm.baseDir, schema.Name+".schema.json")
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}
	return os.WriteFile(schemaFile, data, 0644)
}

// LoadSchema loads a table schema from disk
func (sm *SchemaManager) LoadSchema(tableName string) (*TableSchema, error) {
	schemaFile := filepath.Join(sm.baseDir, tableName+".schema.json")
	data, err := os.ReadFile(schemaFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}

	var schema TableSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}

	return &schema, nil
}

// ListSchemas returns all table names
func (sm *SchemaManager) ListSchemas() ([]string, error) {
	files, err := os.ReadDir(sm.baseDir)
	if err != nil {
		return nil, err
	}

	tables := []string{}
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".schema.json" {
			name := file.Name()[:len(file.Name())-len(".schema.json")]
			tables = append(tables, name)
		}
	}

	return tables, nil
}

// SchemaExists checks if a schema exists
func (sm *SchemaManager) SchemaExists(tableName string) bool {
	schemaFile := filepath.Join(sm.baseDir, tableName+".schema.json")
	_, err := os.Stat(schemaFile)
	return err == nil
}
