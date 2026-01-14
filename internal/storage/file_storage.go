package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"rdbms/internal/types"
)

// FileStorage implements StorageEngine using file-based storage
type FileStorage struct {
	baseDir       string
	schemaManager *SchemaManager
	indexes       map[string]*BTreeIndex // table -> index for primary key
	dataFiles     map[string]*os.File    // table -> data file
}

// NewFileStorage creates a new file-based storage engine
func NewFileStorage(baseDir string) (*FileStorage, error) {
	os.MkdirAll(baseDir, 0755)
	fs := &FileStorage{
		baseDir:       baseDir,
		schemaManager: NewSchemaManager(baseDir),
		indexes:       make(map[string]*BTreeIndex),
		dataFiles:     make(map[string]*os.File),
	}
	return fs, nil
}

// CreateTable creates a new table
func (fs *FileStorage) CreateTable(schema *TableSchema) error {
	if fs.schemaManager.SchemaExists(schema.Name) {
		return fmt.Errorf("table %s already exists", schema.Name)
	}

	// Save schema
	if err := fs.schemaManager.SaveSchema(schema); err != nil {
		return err
	}

	// Create data file
	dataPath := filepath.Join(fs.baseDir, schema.Name+".data")
	file, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to create data file: %w", err)
	}
	fs.dataFiles[schema.Name] = file

	// Create index for primary key if exists
	if len(schema.PrimaryKey) > 0 {
		indexPath := filepath.Join(fs.baseDir, schema.Name+".idx")
		index, err := NewBTreeIndex(indexPath)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
		fs.indexes[schema.Name] = index
	}

	return nil
}

// Insert inserts a row into a table
func (fs *FileStorage) Insert(tableName string, row *Row) error {
	schema, err := fs.schemaManager.LoadSchema(tableName)
	if err != nil {
		return err
	}

	// Validate row
	if len(row.Values) != len(schema.Columns) {
		return fmt.Errorf("row has %d values, expected %d", len(row.Values), len(schema.Columns))
	}

	// Check primary key constraint
	if len(schema.PrimaryKey) > 0 {
		// For simplicity, assume single-column primary key
		pkColName := schema.PrimaryKey[0]
		var pkValue *types.Value
		for i, col := range schema.Columns {
			if col.Name == pkColName {
				pkValue = row.Values[i]
				break
			}
		}

		if pkValue == nil || pkValue.IsNull {
			return fmt.Errorf("primary key cannot be NULL")
		}

		// Check if key already exists
		if index, ok := fs.indexes[tableName]; ok && index != nil {
			_, exists := index.Search(pkValue)
			if exists {
				return fmt.Errorf("duplicate primary key value")
			}
		} else if len(schema.PrimaryKey) > 0 {
			// Index not loaded, try to load it
			indexPath := filepath.Join(fs.baseDir, tableName+".idx")
			index, err := NewBTreeIndex(indexPath)
			if err == nil {
				fs.indexes[tableName] = index
				_, exists := index.Search(pkValue)
				if exists {
					return fmt.Errorf("duplicate primary key value")
				}
			}
		}
	}

	// Write row to data file
	file := fs.dataFiles[tableName]
	if file == nil {
		// File not open, open it
		dataPath := filepath.Join(fs.baseDir, tableName+".data")
		var err error
		file, err = os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("failed to open data file: %w", err)
		}
		fs.dataFiles[tableName] = file
	}
	pos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	rowData, err := fs.serializeRow(row, schema)
	if err != nil {
		return err
	}

	// Write row length first
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(rowData)))
	if _, err := file.Write(lengthBuf); err != nil {
		return err
	}

	if _, err := file.Write(rowData); err != nil {
		return err
	}

	// Update index
	if len(schema.PrimaryKey) > 0 {
		pkColName := schema.PrimaryKey[0]
		var pkValue *types.Value
		for i, col := range schema.Columns {
			if col.Name == pkColName {
				pkValue = row.Values[i]
				break
			}
		}
		index, ok := fs.indexes[tableName]
		if !ok || index == nil {
			// Load index if it exists
			indexPath := filepath.Join(fs.baseDir, tableName+".idx")
			var err error
			index, err = NewBTreeIndex(indexPath)
			if err == nil {
				fs.indexes[tableName] = index
			}
		}
		if index != nil {
			if err := index.Insert(pkValue, pos); err != nil {
				return err
			}
		}
	}

	return nil
}

// Select selects rows from a table
func (fs *FileStorage) Select(tableName string, filter func(*Row) bool) ([]*Row, error) {
	schema, err := fs.schemaManager.LoadSchema(tableName)
	if err != nil {
		return nil, err
	}

	file := fs.dataFiles[tableName]
	if file == nil {
		dataPath := filepath.Join(fs.baseDir, tableName+".data")
		file, err = os.OpenFile(dataPath, os.O_RDONLY, 0644)
		if err != nil {
			return nil, err
		}
		defer file.Close()
	}

	rows := []*Row{}
	
	// Ensure we're at the start of the file
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to start: %w", err)
	}

	for {
		lengthBuf := make([]byte, 4)
		n, err := file.Read(lengthBuf)
		if n == 0 {
			if err != nil && err.Error() != "EOF" {
				break
			}
			break
		}
		if err != nil && n < 4 {
			break
		}

		length := binary.BigEndian.Uint32(lengthBuf)
		if length == 0 {
			break
		}
		
		rowData := make([]byte, length)
		n, err = file.Read(rowData)
		if n == 0 || err != nil {
			if err != nil && err.Error() != "EOF" {
				break
			}
			break
		}
		if n < int(length) {
			// Didn't read full row, skip it
			continue
		}

		row, err := fs.deserializeRow(rowData, schema)
		if err != nil {
			// Skip corrupted rows
			continue
		}

		if filter == nil || filter(row) {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

// Update updates rows in a table
func (fs *FileStorage) Update(tableName string, filter func(*Row) bool, update func(*Row) *Row) (int, error) {
	_, err := fs.schemaManager.LoadSchema(tableName)
	if err != nil {
		return 0, err
	}

	// For simplicity, read all rows, filter, update, and rewrite
	rows, err := fs.Select(tableName, filter)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, row := range rows {
		newRow := update(row)
		if newRow != nil {
			// In a real implementation, we'd update in place
			// For now, we'll delete and re-insert
			updated++
		}
	}

	// Simplified: delete all matching rows and re-insert updated ones
	// In production, this would be more efficient
	allRows, _ := fs.Select(tableName, nil)
	fs.deleteAllRows(tableName)

	for _, row := range allRows {
		if filter != nil && filter(row) {
			newRow := update(row)
			if newRow != nil {
				fs.Insert(tableName, newRow)
			}
		} else {
			fs.Insert(tableName, row)
		}
	}

	return updated, nil
}

func (fs *FileStorage) deleteAllRows(tableName string) error {
	file := fs.dataFiles[tableName]
	if file != nil {
		file.Close()
	}
	dataPath := filepath.Join(fs.baseDir, tableName+".data")
	os.Remove(dataPath)
	file, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	fs.dataFiles[tableName] = file

	// Recreate index
	if index, ok := fs.indexes[tableName]; ok && index != nil {
		index.Close()
		indexPath := filepath.Join(fs.baseDir, tableName+".idx")
		os.Remove(indexPath)
		schema, _ := fs.schemaManager.LoadSchema(tableName)
		if len(schema.PrimaryKey) > 0 {
			newIndex, err := NewBTreeIndex(indexPath)
			if err != nil {
				return err
			}
			fs.indexes[tableName] = newIndex
		}
	}
	return nil
}

// Delete deletes rows from a table
func (fs *FileStorage) Delete(tableName string, filter func(*Row) bool) (int, error) {
	// Simplified: read all, filter, rewrite
	allRows, err := fs.Select(tableName, nil)
	if err != nil {
		return 0, err
	}

	deleted := 0
	remaining := []*Row{}
	for _, row := range allRows {
		if filter == nil || filter(row) {
			deleted++
		} else {
			remaining = append(remaining, row)
		}
	}

	fs.deleteAllRows(tableName)
	for _, row := range remaining {
		fs.Insert(tableName, row)
	}

	return deleted, nil
}

// GetSchema returns the schema for a table
func (fs *FileStorage) GetSchema(tableName string) (*TableSchema, error) {
	return fs.schemaManager.LoadSchema(tableName)
}

// ListTables returns all table names
func (fs *FileStorage) ListTables() ([]string, error) {
	return fs.schemaManager.ListSchemas()
}

// Close closes all open files
func (fs *FileStorage) Close() error {
	for _, file := range fs.dataFiles {
		file.Close()
	}
	for _, index := range fs.indexes {
		index.Close()
	}
	return nil
}

func (fs *FileStorage) serializeRow(row *Row, schema *TableSchema) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	for i, val := range row.Values {
		if val == nil {
			return nil, fmt.Errorf("column %s has nil value", schema.Columns[i].Name)
		}
		// Ensure value type matches column type for serialization
		valToSerialize := val
		if val.Type != schema.Columns[i].Type {
			// TypeVarchar and TypeText are compatible
			if (val.Type == types.TypeText && schema.Columns[i].Type == types.TypeVarchar) ||
				(val.Type == types.TypeVarchar && schema.Columns[i].Type == types.TypeText) {
				// Create a copy with correct type for serialization
				valToSerialize = &types.Value{
					Type:   schema.Columns[i].Type,
					Data:   val.Data,
					IsNull: val.IsNull,
				}
			}
		}
		valData, err := valToSerialize.Serialize()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize column %s (type %d, isNull: %v): %w", 
				schema.Columns[i].Name, val.Type, val.IsNull, err)
		}
		lengthBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lengthBuf, uint32(len(valData)))
		buf = append(buf, lengthBuf...)
		buf = append(buf, valData...)
	}
	return buf, nil
}

func (fs *FileStorage) deserializeRow(data []byte, schema *TableSchema) (*Row, error) {
	row := &Row{Values: make([]*types.Value, len(schema.Columns))}
	offset := 0

	for i, col := range schema.Columns {
		if offset+4 > len(data) {
			return nil, fmt.Errorf("insufficient data for column %s", col.Name)
		}

		length := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if offset+int(length) > len(data) {
			return nil, fmt.Errorf("insufficient data for column %s", col.Name)
		}

		valData := data[offset : offset+int(length)]
		offset += int(length)

		val, err := types.DeserializeValue(col.Type, valData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize column %s: %w", col.Name, err)
		}

		row.Values[i] = val
	}

	return row, nil
}
