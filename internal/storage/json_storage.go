package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"rdbms/internal/types"
)

// JSONStorage implements StorageEngine using JSON files
type JSONStorage struct {
	baseDir       string
	schemaManager *SchemaManager
	tables        map[string]*JSONTable // table name -> table data
}

// JSONTable represents a table stored in JSON format
type JSONTable struct {
	Name    string
	Schema  *TableSchema
	Rows    []*Row
}

// NewJSONStorage creates a new JSON-based storage engine
func NewJSONStorage(baseDir string) (*JSONStorage, error) {
	os.MkdirAll(baseDir, 0755)
	js := &JSONStorage{
		baseDir:       baseDir,
		schemaManager: NewSchemaManager(baseDir),
		tables:        make(map[string]*JSONTable),
	}
	
	// Load existing tables
	tables, err := js.schemaManager.ListSchemas()
	if err == nil {
		for _, tableName := range tables {
			// #region agent log
			if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
				json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "init", "hypothesisId": "D", "location": "json_storage.go:38", "message": "Loading table during init", "data": map[string]interface{}{"tableName": tableName}, "timestamp": time.Now().UnixMilli()})
				f.Close()
			}
			// #endregion
			if err := js.loadTable(tableName); err != nil {
				// Continue if table can't be loaded
				continue
			}
			// #region agent log
			if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
				table := js.tables[tableName]
				rowCount := 0
				if table != nil {
					rowCount = len(table.Rows)
				}
				json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "init", "hypothesisId": "D", "location": "json_storage.go:42", "message": "Table loaded during init", "data": map[string]interface{}{"tableName": tableName, "rowCount": rowCount}, "timestamp": time.Now().UnixMilli()})
				f.Close()
			}
			// #endregion
		}
	}
	
	return js, nil
}

// loadTable loads a table from JSON file
func (js *JSONStorage) loadTable(tableName string) error {
	tableFile := filepath.Join(js.baseDir, tableName+".json")
	
	schema, err := js.schemaManager.LoadSchema(tableName)
	if err != nil {
		return err
	}
	
	data, err := os.ReadFile(tableFile)
	if err != nil {
		// File doesn't exist yet, create empty table
		js.tables[tableName] = &JSONTable{
			Name:   tableName,
			Schema: schema,
			Rows:   []*Row{},
		}
		return nil
	}
	
	// Parse JSON
	var jsonRows []map[string]interface{}
	if err := json.Unmarshal(data, &jsonRows); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	// Convert JSON rows to Row objects
	rows := make([]*Row, 0, len(jsonRows))
	for _, jsonRow := range jsonRows {
		row := &Row{Values: make([]*types.Value, len(schema.Columns))}
		for i, col := range schema.Columns {
			val, err := js.jsonValueToValue(jsonRow[col.Name], col.Type)
			if err != nil {
				return fmt.Errorf("failed to convert value for column %s: %w", col.Name, err)
			}
			row.Values[i] = val
		}
		rows = append(rows, row)
	}
	
	js.tables[tableName] = &JSONTable{
		Name:   tableName,
		Schema: schema,
		Rows:   rows,
	}
	// #region agent log
	if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "load", "hypothesisId": "D", "location": "json_storage.go:95", "message": "loadTable completed", "data": map[string]interface{}{"tableName": tableName, "rowsLoaded": len(rows)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	
	return nil
}

// saveTable saves a table to JSON file
func (js *JSONStorage) saveTable(tableName string) error {
	// #region agent log
	if f, err := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "save", "hypothesisId": "C", "location": "json_storage.go:99", "message": "saveTable entry", "data": map[string]interface{}{"tableName": tableName}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	table, ok := js.tables[tableName]
	if !ok {
		return fmt.Errorf("table %s not found", tableName)
	}
	
	tableFile := filepath.Join(js.baseDir, tableName+".json")
	
	// Convert rows to JSON
	jsonRows := make([]map[string]interface{}, len(table.Rows))
	for i, row := range table.Rows {
		jsonRow := make(map[string]interface{})
		for j, col := range table.Schema.Columns {
			if j < len(row.Values) && row.Values[j] != nil {
				jsonVal := js.valueToJSONValue(row.Values[j])
				jsonRow[col.Name] = jsonVal
			} else {
				jsonRow[col.Name] = nil
			}
		}
		jsonRows[i] = jsonRow
	}
	
	// #region agent log
	if f, err := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "save", "hypothesisId": "C", "location": "json_storage.go:121", "message": "Before WriteFile", "data": map[string]interface{}{"tableName": tableName, "tableFile": tableFile, "rowCount": len(jsonRows)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	
	data, err := json.MarshalIndent(jsonRows, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	
	err = os.WriteFile(tableFile, data, 0644)
	if err == nil {
		// Ensure data is written to disk by opening and syncing the file
		if file, syncErr := os.OpenFile(tableFile, os.O_RDWR, 0644); syncErr == nil {
			file.Sync()
			file.Close()
		}
	}
	// #region agent log
	if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "save", "hypothesisId": "C", "location": "json_storage.go:127", "message": "WriteFile result", "data": map[string]interface{}{"tableName": tableName, "tableFile": tableFile, "dataLen": len(data), "err": fmt.Sprintf("%v", err)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	return err
}

// jsonValueToValue converts a JSON value to a types.Value
func (js *JSONStorage) jsonValueToValue(jsonVal interface{}, dataType types.DataType) (*types.Value, error) {
	if jsonVal == nil {
		return &types.Value{Type: dataType, IsNull: true}, nil
	}
	
	return types.NewValue(dataType, jsonVal)
}

// valueToJSONValue converts a types.Value to a JSON-compatible value
func (js *JSONStorage) valueToJSONValue(val *types.Value) interface{} {
	if val == nil || val.IsNull {
		return nil
	}
	
	switch val.Type {
	case types.TypeInteger:
		if v, ok := val.Data.(int64); ok {
			return v
		}
		return val.String()
	case types.TypeFloat:
		if v, ok := val.Data.(float64); ok {
			return v
		}
		return val.String()
	case types.TypeVarchar, types.TypeText:
		if v, ok := val.Data.(string); ok {
			return v
		}
		// Fallback to String() method if type assertion fails
		return val.String()
	case types.TypeBoolean:
		if v, ok := val.Data.(bool); ok {
			return v
		}
		return val.String()
	case types.TypeDate, types.TypeTimestamp:
		if t, ok := val.Data.(time.Time); ok {
			return t.Format("2006-01-02 15:04:05")
		}
		return val.String()
	default:
		return fmt.Sprintf("%v", val.Data)
	}
}

// CreateTable creates a new table
func (js *JSONStorage) CreateTable(schema *TableSchema) error {
	if js.schemaManager.SchemaExists(schema.Name) {
		return fmt.Errorf("table %s already exists", schema.Name)
	}
	
	// Save schema
	if err := js.schemaManager.SaveSchema(schema); err != nil {
		return err
	}
	
	// Create empty table
	js.tables[schema.Name] = &JSONTable{
		Name:   schema.Name,
		Schema: schema,
		Rows:   []*Row{},
	}
	
	return js.saveTable(schema.Name)
}

// Insert inserts a row into a table
func (js *JSONStorage) Insert(tableName string, row *Row) error {
	// #region agent log
	if f, err := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "insert", "hypothesisId": "A", "location": "json_storage.go:199", "message": "Insert entry", "data": map[string]interface{}{"tableName": tableName, "rowValues": len(row.Values), "tablesInMemory": len(js.tables)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	table, ok := js.tables[tableName]
	if !ok {
		// #region agent log
		if f, err := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "insert", "hypothesisId": "B", "location": "json_storage.go:202", "message": "Table not in memory, loading", "data": map[string]interface{}{"tableName": tableName}, "timestamp": time.Now().UnixMilli()})
			f.Close()
		}
		// #endregion
		// Try to load table
		if err := js.loadTable(tableName); err != nil {
			return fmt.Errorf("table %s not found", tableName)
		}
		table = js.tables[tableName]
	}
	
	// Validate row
	if len(row.Values) != len(table.Schema.Columns) {
		return fmt.Errorf("row has %d values, expected %d", len(row.Values), len(table.Schema.Columns))
	}
	
	// Check primary key constraint
	if len(table.Schema.PrimaryKey) > 0 {
		for _, pkCol := range table.Schema.PrimaryKey {
			var pkValue *types.Value
			for i, col := range table.Schema.Columns {
				if col.Name == pkCol {
					pkValue = row.Values[i]
					break
				}
			}
			
			if pkValue == nil || pkValue.IsNull {
				return fmt.Errorf("primary key cannot be NULL")
			}
			
			// Check for duplicates
			for _, existingRow := range table.Rows {
				for i, col := range table.Schema.Columns {
					if col.Name == pkCol {
						existingVal := existingRow.Values[i]
						if existingVal != nil && !existingVal.IsNull {
							if cmp, _ := pkValue.Compare(existingVal); cmp == 0 {
								return fmt.Errorf("duplicate primary key value")
							}
						}
						break
					}
				}
			}
		}
	}
	
	// Add row (make a copy to avoid issues)
	newRow := &Row{Values: make([]*types.Value, len(row.Values))}
	for i, val := range row.Values {
		if val != nil {
			newRow.Values[i] = &types.Value{
				Type:   val.Type,
				Data:   val.Data,
				IsNull: val.IsNull,
			}
		} else {
			newRow.Values[i] = &types.Value{Type: table.Schema.Columns[i].Type, IsNull: true}
		}
	}
	table.Rows = append(table.Rows, newRow)
	// #region agent log
	if f, err := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "insert", "hypothesisId": "C", "location": "json_storage.go:259", "message": "Row added to memory", "data": map[string]interface{}{"tableName": tableName, "totalRows": len(table.Rows)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	
	err := js.saveTable(tableName)
	// #region agent log
	if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
		json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "insert", "hypothesisId": "C", "location": "json_storage.go:261", "message": "saveTable result", "data": map[string]interface{}{"tableName": tableName, "err": fmt.Sprintf("%v", err)}, "timestamp": time.Now().UnixMilli()})
		f.Close()
	}
	// #endregion
	return err
}

// Select selects rows from a table
func (js *JSONStorage) Select(tableName string, filter func(*Row) bool) ([]*Row, error) {
	// Always reload table from disk to get latest data (real-time updates)
	// This ensures changes made by REPL or other processes are visible
	if err := js.loadTable(tableName); err != nil {
		return nil, fmt.Errorf("table %s not found: %w", tableName, err)
	}
	
	table, ok := js.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("table %s not found", tableName)
	}
	
	rows := []*Row{}
	for _, row := range table.Rows {
		if filter == nil || filter(row) {
			rows = append(rows, row)
		}
	}
	
	return rows, nil
}

// Update updates rows in a table
func (js *JSONStorage) Update(tableName string, filter func(*Row) bool, update func(*Row) *Row) (int, error) {
	// Always reload table from disk to get latest data (real-time updates)
	if err := js.loadTable(tableName); err != nil {
		return 0, fmt.Errorf("table %s not found: %w", tableName, err)
	}
	
	table, ok := js.tables[tableName]
	if !ok {
		return 0, fmt.Errorf("table %s not found", tableName)
	}
	
	updated := 0
	for i, row := range table.Rows {
		if filter == nil || filter(row) {
			newRow := update(row)
			if newRow != nil {
				table.Rows[i] = newRow
				updated++
			}
		}
	}
	
	if updated > 0 {
		if err := js.saveTable(tableName); err != nil {
			return updated, err
		}
	}
	
	return updated, nil
}

// Delete deletes rows from a table
func (js *JSONStorage) Delete(tableName string, filter func(*Row) bool) (int, error) {
	// Always reload table from disk to get latest data (real-time updates)
	if err := js.loadTable(tableName); err != nil {
		return 0, fmt.Errorf("table %s not found: %w", tableName, err)
	}
	
	table, ok := js.tables[tableName]
	if !ok {
		return 0, fmt.Errorf("table %s not found", tableName)
	}
	
	deleted := 0
	newRows := []*Row{}
	for _, row := range table.Rows {
		if filter == nil || filter(row) {
			deleted++
		} else {
			newRows = append(newRows, row)
		}
	}
	
	table.Rows = newRows
	
	if deleted > 0 {
		if err := js.saveTable(tableName); err != nil {
			return deleted, err
		}
	}
	
	return deleted, nil
}

// GetSchema returns the schema for a table
func (js *JSONStorage) GetSchema(tableName string) (*TableSchema, error) {
	return js.schemaManager.LoadSchema(tableName)
}

// ListTables returns all table names
func (js *JSONStorage) ListTables() ([]string, error) {
	return js.schemaManager.ListSchemas()
}

// Close closes the storage (no-op for JSON storage)
func (js *JSONStorage) Close() error {
	// Save all tables
	for tableName := range js.tables {
		js.saveTable(tableName)
	}
	return nil
}
