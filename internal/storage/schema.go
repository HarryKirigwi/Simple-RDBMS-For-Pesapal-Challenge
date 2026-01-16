package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rdbms/internal/types"
	"time"
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
	// #region agent log
	if f, err := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "init", "hypothesisId": "A", "location": "schema.go:65", "message": "ListSchemas entry", "data": map[string]interface{}{"baseDir": sm.baseDir}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	files, err := os.ReadDir(sm.baseDir)
	// #region agent log
	if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
		fileNames := make([]string, 0, len(files))
		if files != nil {
			for _, f := range files {
				fileNames = append(fileNames, f.Name())
			}
		}
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "init", "hypothesisId": "A", "location": "schema.go:67", "message": "ReadDir result", "data": map[string]interface{}{"baseDir": sm.baseDir, "err": fmt.Sprintf("%v", err), "fileCount": len(fileNames), "files": fileNames}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	if err != nil {
		return nil, err
	}

	tables := []string{}
	for _, file := range files {
		fileName := file.Name()
		// Check if file ends with .schema.json (filepath.Ext only returns last extension)
		// #region agent log
		if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
			ext := filepath.Ext(fileName)
			endsWithSchema := len(fileName) >= len(".schema.json") && fileName[len(fileName)-len(".schema.json"):] == ".schema.json"
			json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "init", "hypothesisId": "A", "location": "schema.go:73", "message": "Checking file extension", "data": map[string]interface{}{"fileName": fileName, "ext": ext, "endsWithSchema": endsWithSchema}, "timestamp": time.Now().UnixMilli()})
			f.Close()
		}
		// #endregion
		if len(fileName) >= len(".schema.json") && fileName[len(fileName)-len(".schema.json"):] == ".schema.json" {
			name := fileName[:len(fileName)-len(".schema.json")]
			tables = append(tables, name)
		}
	}
	// #region agent log
	if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "init", "hypothesisId": "A", "location": "schema.go:79", "message": "ListSchemas exit", "data": map[string]interface{}{"tables": tables, "count": len(tables)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion

	return tables, nil
}

// SchemaExists checks if a schema exists
func (sm *SchemaManager) SchemaExists(tableName string) bool {
	schemaFile := filepath.Join(sm.baseDir, tableName+".schema.json")
	_, err := os.Stat(schemaFile)
	return err == nil
}
