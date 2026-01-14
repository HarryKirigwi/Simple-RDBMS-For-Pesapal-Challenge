package constraint

import (
	"fmt"
	"rdbms/internal/storage"
	"rdbms/internal/types"
)

// ConstraintManager manages and enforces constraints
type ConstraintManager struct {
	storage storage.StorageEngine
}

// NewConstraintManager creates a new constraint manager
func NewConstraintManager(storage storage.StorageEngine) *ConstraintManager {
	return &ConstraintManager{storage: storage}
}

// ValidateInsert validates constraints before inserting a row
func (cm *ConstraintManager) ValidateInsert(tableName string, row *storage.Row) error {
	schema, err := cm.storage.GetSchema(tableName)
	if err != nil {
		return err
	}

	// Validate primary key
	if len(schema.PrimaryKey) > 0 {
		if err := cm.validatePrimaryKey(tableName, schema, row); err != nil {
			return err
		}
	}

	// Validate unique constraints
	for _, uniqueCols := range schema.UniqueKeys {
		if err := cm.validateUnique(tableName, schema, row, uniqueCols); err != nil {
			return err
		}
	}

	// Validate NOT NULL constraints
	for i, col := range schema.Columns {
		if !col.Nullable && (i >= len(row.Values) || row.Values[i] == nil || row.Values[i].IsNull) {
			return fmt.Errorf("column %s cannot be NULL", col.Name)
		}
	}

	return nil
}

// ValidateUpdate validates constraints before updating a row
func (cm *ConstraintManager) ValidateUpdate(tableName string, oldRow, newRow *storage.Row) error {
	schema, err := cm.storage.GetSchema(tableName)
	if err != nil {
		return err
	}

	// Check if primary key is being changed
	if len(schema.PrimaryKey) > 0 {
		for _, pkCol := range schema.PrimaryKey {
			var oldIdx, newIdx int
			for i, col := range schema.Columns {
				if col.Name == pkCol {
					oldIdx = i
					newIdx = i
					break
				}
			}

			if oldIdx < len(oldRow.Values) && newIdx < len(newRow.Values) {
				oldVal := oldRow.Values[oldIdx]
				newVal := newRow.Values[newIdx]

				if oldVal != nil && newVal != nil && !oldVal.IsNull && !newVal.IsNull {
					if cmp, _ := oldVal.Compare(newVal); cmp != 0 {
						// Primary key is being changed, check if new value already exists
						if err := cm.validatePrimaryKey(tableName, schema, newRow); err != nil {
							return fmt.Errorf("cannot update primary key: %w", err)
						}
					}
				}
			}
		}
	}

	// Validate unique constraints
	for _, uniqueCols := range schema.UniqueKeys {
		// Check if any unique column is being changed
		changed := false
		for _, uniqueCol := range uniqueCols {
			var oldIdx, newIdx int
			for i, col := range schema.Columns {
				if col.Name == uniqueCol {
					oldIdx = i
					newIdx = i
					break
				}
			}

			if oldIdx < len(oldRow.Values) && newIdx < len(newRow.Values) {
				oldVal := oldRow.Values[oldIdx]
				newVal := newRow.Values[newIdx]

				if (oldVal == nil || oldVal.IsNull) != (newVal == nil || newVal.IsNull) {
					changed = true
					break
				}
				if oldVal != nil && newVal != nil && !oldVal.IsNull && !newVal.IsNull {
					if cmp, _ := oldVal.Compare(newVal); cmp != 0 {
						changed = true
						break
					}
				}
			}
		}

		if changed {
			if err := cm.validateUnique(tableName, schema, newRow, uniqueCols); err != nil {
				return err
			}
		}
	}

	// Validate NOT NULL constraints
	for i, col := range schema.Columns {
		if !col.Nullable && (i >= len(newRow.Values) || newRow.Values[i] == nil || newRow.Values[i].IsNull) {
			return fmt.Errorf("column %s cannot be NULL", col.Name)
		}
	}

	return nil
}

func (cm *ConstraintManager) validatePrimaryKey(tableName string, schema *storage.TableSchema, row *storage.Row) error {
	// Check that primary key is not NULL
	for _, pkCol := range schema.PrimaryKey {
		var pkIdx int
		var found bool
		for i, col := range schema.Columns {
			if col.Name == pkCol {
				pkIdx = i
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("primary key column %s not found in schema", pkCol)
		}

		if pkIdx >= len(row.Values) || row.Values[pkIdx] == nil || row.Values[pkIdx].IsNull {
			return fmt.Errorf("primary key column %s cannot be NULL", pkCol)
		}
	}

	// Check uniqueness by scanning existing rows
	// For single-column primary key, we can use index if available
	if len(schema.PrimaryKey) == 1 {
		pkColName := schema.PrimaryKey[0]
		var pkValue *types.Value
		for i, col := range schema.Columns {
			if col.Name == pkColName {
				pkValue = row.Values[i]
				break
			}
		}

		if pkValue != nil && !pkValue.IsNull {
			// Check if key already exists
			existingRows, err := cm.storage.Select(tableName, func(r *storage.Row) bool {
				var rowPkValue *types.Value
				for i, col := range schema.Columns {
					if col.Name == pkColName {
						rowPkValue = r.Values[i]
						break
					}
				}
				if rowPkValue == nil || rowPkValue.IsNull {
					return false
				}
				cmp, _ := pkValue.Compare(rowPkValue)
				return cmp == 0
			})

			if err == nil && len(existingRows) > 0 {
				return fmt.Errorf("duplicate primary key value")
			}
		}
	}

	return nil
}

func (cm *ConstraintManager) validateUnique(tableName string, schema *storage.TableSchema, row *storage.Row, uniqueCols []string) error {
	// Build values for unique columns
	uniqueValues := make([]*types.Value, len(uniqueCols))
	for i, uniqueCol := range uniqueCols {
		var found bool
		for j, col := range schema.Columns {
			if col.Name == uniqueCol {
				if j < len(row.Values) {
					uniqueValues[i] = row.Values[j]
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unique column %s not found in schema", uniqueCol)
		}
	}

	// Check if all unique values are NULL (NULLs are considered distinct in SQL)
	allNull := true
	for _, val := range uniqueValues {
		if val != nil && !val.IsNull {
			allNull = false
			break
		}
	}
	if allNull {
		return nil // NULL values don't violate unique constraint
	}

	// Check uniqueness by scanning existing rows
	existingRows, err := cm.storage.Select(tableName, func(r *storage.Row) bool {
		for i, uniqueCol := range uniqueCols {
			var rowValue *types.Value
			for j, col := range schema.Columns {
				if col.Name == uniqueCol {
					if j < len(r.Values) {
						rowValue = r.Values[j]
					}
					break
				}
			}

			val := uniqueValues[i]
			if (val == nil || val.IsNull) != (rowValue == nil || rowValue.IsNull) {
				return false
			}
			if val != nil && rowValue != nil && !val.IsNull && !rowValue.IsNull {
				cmp, _ := val.Compare(rowValue)
				if cmp != 0 {
					return false
				}
			}
		}
		return true
	})

	if err == nil && len(existingRows) > 0 {
		return fmt.Errorf("duplicate unique key value")
	}

	return nil
}
