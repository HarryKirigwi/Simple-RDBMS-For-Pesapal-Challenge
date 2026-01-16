package query

import (
	"encoding/json"
	"fmt"
	"os"
	"rdbms/internal/constraint"
	"rdbms/internal/parser"
	"rdbms/internal/storage"
	"rdbms/internal/types"
	"strings"
	"time"
)

// Executor executes SQL statements
type Executor struct {
	storage   storage.StorageEngine
	constraint *constraint.ConstraintManager
}

// NewExecutor creates a new query executor
func NewExecutor(storage storage.StorageEngine) *Executor {
	return &Executor{
		storage:    storage,
		constraint: constraint.NewConstraintManager(storage),
	}
}

// Execute executes a parsed statement
func (e *Executor) Execute(stmt parser.Statement) (*Result, error) {
	if stmt == nil {
		return nil, fmt.Errorf("statement is nil")
	}
	switch s := stmt.(type) {
	case *parser.CreateTableStmt:
		if s == nil {
			return nil, fmt.Errorf("CreateTableStmt is nil")
		}
		return e.executeCreateTable(s)
	case *parser.InsertStmt:
		if s == nil {
			return nil, fmt.Errorf("InsertStmt is nil")
		}
		return e.executeInsert(s)
	case *parser.SelectStmt:
		if s == nil {
			return nil, fmt.Errorf("SelectStmt is nil")
		}
		return e.executeSelect(s)
	case *parser.UpdateStmt:
		if s == nil {
			return nil, fmt.Errorf("UpdateStmt is nil")
		}
		return e.executeUpdate(s)
	case *parser.DeleteStmt:
		if s == nil {
			return nil, fmt.Errorf("DeleteStmt is nil")
		}
		return e.executeDelete(s)
	default:
		return nil, fmt.Errorf("unsupported statement type: %T", stmt)
	}
}

func (e *Executor) executeCreateTable(stmt *parser.CreateTableStmt) (*Result, error) {
	schema := &storage.TableSchema{
		Name:       stmt.TableName,
		PrimaryKey: stmt.PrimaryKey,
		UniqueKeys: stmt.UniqueKeys,
	}

	schema.Columns = make([]storage.ColumnDef, len(stmt.Columns))
	for i, col := range stmt.Columns {
		schema.Columns[i] = storage.ColumnDef{
			Name:     col.Name,
			Type:     col.Type,
			Size:     col.Size,
			Nullable: col.Nullable,
		}
	}

	if err := e.storage.CreateTable(schema); err != nil {
		return nil, err
	}

	return &Result{Message: fmt.Sprintf("Table %s created", stmt.TableName)}, nil
}

func (e *Executor) executeInsert(stmt *parser.InsertStmt) (*Result, error) {
	schema, err := e.storage.GetSchema(stmt.TableName)
	if err != nil {
		return nil, err
	}

	inserted := 0
	for _, valueList := range stmt.Values {
		row := &storage.Row{Values: make([]*types.Value, len(schema.Columns))}

		// Map values to columns
		if len(stmt.Columns) > 0 {
			// Insert with column list
			colMap := make(map[string]int)
			for i, colName := range stmt.Columns {
				colMap[colName] = i
			}

			for i, col := range schema.Columns {
				if idx, ok := colMap[col.Name]; ok && idx < len(valueList) {
					row.Values[i] = valueList[idx]
				} else {
					// Use NULL for unspecified columns
					row.Values[i] = &types.Value{Type: col.Type, IsNull: true}
				}
			}
		} else {
			// Insert without column list - values in order
			for i, val := range valueList {
				if i < len(row.Values) {
					// Always ensure value type matches column type
					colType := schema.Columns[i].Type
					if val.IsNull {
						row.Values[i] = &types.Value{Type: colType, IsNull: true}
					} else if val.Type != colType {
						// TypeVarchar and TypeText are compatible - just update the type
						if (val.Type == types.TypeText && colType == types.TypeVarchar) ||
							(val.Type == types.TypeVarchar && colType == types.TypeText) {
							// Create a copy with the correct type
							newVal := &types.Value{
								Type:   colType,
								Data:   val.Data,
								IsNull: val.IsNull,
							}
							row.Values[i] = newVal
						} else {
							// Try to convert the value to the column type
							convertedVal, err := types.NewValue(colType, val.Data)
							if err == nil {
								row.Values[i] = convertedVal
							} else {
								// If conversion fails, use original value but update type if compatible
								if (val.Type == types.TypeText || val.Type == types.TypeVarchar) &&
									(colType == types.TypeText || colType == types.TypeVarchar) {
									val.Type = colType
								}
								row.Values[i] = val
							}
						}
					} else {
						row.Values[i] = val
					}
				}
			}
			// Fill remaining with NULL
			for i := len(valueList); i < len(row.Values); i++ {
				row.Values[i] = &types.Value{Type: schema.Columns[i].Type, IsNull: true}
			}
		}

		// Validate constraints
		if err := e.constraint.ValidateInsert(stmt.TableName, row); err != nil {
			return nil, err
		}

		if err := e.storage.Insert(stmt.TableName, row); err != nil {
			return nil, err
		}
		inserted++
	}

	return &Result{Message: fmt.Sprintf("%d row(s) inserted", inserted)}, nil
}

func (e *Executor) executeSelect(stmt *parser.SelectStmt) (*Result, error) {
	rows, err := e.storage.Select(stmt.From, nil)
	if err != nil {
		return nil, err
	}

	// Apply WHERE filter
	if stmt.Where != nil {
		filtered := []*storage.Row{}
		for _, row := range rows {
			// #region agent log
			func() {
				logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				defer logFile.Close()
				whereResult := e.evaluateExpression(stmt.Where, row, stmt.From)
				logData, _ := json.Marshal(map[string]interface{}{
					"sessionId":    "debug-session",
					"runId":        "run1",
					"hypothesisId": "D",
					"location":     "executor.go:161",
					"message":      "SELECT WHERE filter evaluation",
					"data": map[string]interface{}{
						"result": whereResult,
						"rowId":  func() interface{} { if len(row.Values) > 0 && row.Values[0] != nil { return row.Values[0].Data }; return nil }(),
					},
					"timestamp": time.Now().UnixMilli(),
				})
				logFile.Write(append(logData, '\n'))
			}()
			// #endregion
			if e.evaluateExpression(stmt.Where, row, stmt.From) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	// Handle JOINs
	if len(stmt.Joins) > 0 {
		joinedRows, err := e.executeJoins(rows, stmt.From, stmt.Joins)
		if err != nil {
			return nil, err
		}
		rows = joinedRows
	}

	// Project columns
	// Build schema map for all tables (FROM + JOINs)
	schemas := make(map[string]*storage.TableSchema)
	fromSchema, err := e.storage.GetSchema(stmt.From)
	if err != nil {
		return nil, err
	}
	schemas[stmt.From] = fromSchema
	
	// Get schemas for all joined tables
	for _, join := range stmt.Joins {
		joinSchema, err := e.storage.GetSchema(join.Table)
		if err != nil {
			return nil, err
		}
		schemas[join.Table] = joinSchema
	}

	// Check if we have aggregate functions
	hasAggregates := false
	for _, col := range stmt.Columns {
		if col.Function != "" {
			hasAggregates = true
			break
		}
	}

	resultRows := []*storage.Row{}
	
	if hasAggregates {
		// Handle aggregate functions - return single row with aggregate results
		resultRow := &storage.Row{Values: []*types.Value{}}
		
		for _, col := range stmt.Columns {
			if col.Function != "" {
				// Compute aggregate
				var aggValue *types.Value
				var err error
				
				if col.Function == "COUNT" && col.ArgColumn == "*" {
					// COUNT(*)
					aggValue = &types.Value{Type: types.TypeInteger, Data: int64(len(rows)), IsNull: false}
				} else {
					// Find column index for aggregate argument
					var colIdx int
					found := false
					
					if strings.Contains(col.ArgColumn, ".") {
						parts := strings.Split(col.ArgColumn, ".")
						if len(parts) == 2 {
							tableName, columnName := parts[0], parts[1]
							if schema, ok := schemas[tableName]; ok {
								for i, c := range schema.Columns {
									if c.Name == columnName {
										colIdx = i
										found = true
										break
									}
								}
							}
						}
					} else {
						// Search in FROM table first
						fromSchema := schemas[stmt.From]
						for i, c := range fromSchema.Columns {
							if c.Name == col.ArgColumn {
								colIdx = i
								found = true
								break
							}
						}
					}
					
					if !found {
						return nil, fmt.Errorf("column %s not found for aggregate function", col.ArgColumn)
					}
					
					// Compute aggregate over all rows
					aggValue, err = e.computeAggregate(col.Function, rows, colIdx)
					if err != nil {
						return nil, err
					}
				}
				
				resultRow.Values = append(resultRow.Values, aggValue)
			} else {
				// Non-aggregate column - not allowed with aggregates
				return nil, fmt.Errorf("cannot mix aggregate and non-aggregate columns")
			}
		}
		
		resultRows = append(resultRows, resultRow)
	} else {
		// Regular column projection
		for _, row := range rows {
			resultRow := &storage.Row{Values: []*types.Value{}}

			if len(stmt.Columns) == 1 && stmt.Columns[0].IsStar {
				// SELECT *
				resultRow.Values = append(resultRow.Values, row.Values...)
			} else {
				// SELECT specific columns
				for _, col := range stmt.Columns {
					var colIdx int
					found := false
				
				// Handle qualified column names (table.column)
				if strings.Contains(col.Name, ".") {
					parts := strings.Split(col.Name, ".")
					if len(parts) == 2 {
						tableName, columnName := parts[0], parts[1]
						if schema, ok := schemas[tableName]; ok {
							// Find column in the specified table
							for i, schemaCol := range schema.Columns {
								if schemaCol.Name == columnName {
									// Calculate offset in joined row
									offset := 0
									if tableName == stmt.From {
										colIdx = i
									} else {
										// Find position in joined row
										offset = len(fromSchema.Columns)
										for _, join := range stmt.Joins {
											if join.Table == tableName {
												colIdx = offset + i
												break
											}
											joinSchema, _ := e.storage.GetSchema(join.Table)
											offset += len(joinSchema.Columns)
										}
									}
									found = true
									break
								}
							}
						}
					}
				} else {
					// Unqualified column name - search in all tables
					offset := 0
					for tableName, schema := range schemas {
						for i, schemaCol := range schema.Columns {
							if schemaCol.Name == col.Name {
								if tableName == stmt.From {
									colIdx = i
								} else {
									colIdx = offset + i
								}
								found = true
								break
							}
						}
						if found {
							break
						}
						offset += len(schema.Columns)
					}
				}
				
					if found && colIdx < len(row.Values) {
						resultRow.Values = append(resultRow.Values, row.Values[colIdx])
					} else {
						resultRow.Values = append(resultRow.Values, &types.Value{IsNull: true})
					}
				}
			}

			resultRows = append(resultRows, resultRow)
		}
	}

	// Apply ORDER BY (skip for aggregate queries)
	if len(stmt.OrderBy) > 0 && !hasAggregates {
		// Simple sorting - in production, use more efficient algorithm
		for i := 0; i < len(resultRows); i++ {
			for j := i + 1; j < len(resultRows); j++ {
				if e.shouldSwap(resultRows[i], resultRows[j], stmt.OrderBy, fromSchema) {
					resultRows[i], resultRows[j] = resultRows[j], resultRows[i]
				}
			}
		}
	}

	// Apply LIMIT
	if stmt.Limit != nil && *stmt.Limit < len(resultRows) {
		resultRows = resultRows[:*stmt.Limit]
	}

	return &Result{
		Rows:    resultRows,
		Columns: e.getColumnNames(stmt, fromSchema),
	}, nil
}

// computeAggregate computes an aggregate function over rows
func (e *Executor) computeAggregate(funcName string, rows []*storage.Row, colIdx int) (*types.Value, error) {
	if len(rows) == 0 {
		// Empty result set
		if funcName == "COUNT" {
			return &types.Value{Type: types.TypeInteger, Data: int64(0), IsNull: false}, nil
		}
		return &types.Value{IsNull: true}, nil
	}

	switch funcName {
	case "MAX":
		var maxVal *types.Value
		for _, row := range rows {
			if colIdx < len(row.Values) && row.Values[colIdx] != nil && !row.Values[colIdx].IsNull {
				if maxVal == nil {
					maxVal = row.Values[colIdx]
				} else {
					cmp, _ := row.Values[colIdx].Compare(maxVal)
					if cmp > 0 {
						maxVal = row.Values[colIdx]
					}
				}
			}
		}
		if maxVal == nil {
			return &types.Value{IsNull: true}, nil
		}
		return maxVal, nil
	case "MIN":
		var minVal *types.Value
		for _, row := range rows {
			if colIdx < len(row.Values) && row.Values[colIdx] != nil && !row.Values[colIdx].IsNull {
				if minVal == nil {
					minVal = row.Values[colIdx]
				} else {
					cmp, _ := row.Values[colIdx].Compare(minVal)
					if cmp < 0 {
						minVal = row.Values[colIdx]
					}
				}
			}
		}
		if minVal == nil {
			return &types.Value{IsNull: true}, nil
		}
		return minVal, nil
	case "COUNT":
		count := int64(0)
		for _, row := range rows {
			if colIdx < len(row.Values) && row.Values[colIdx] != nil && !row.Values[colIdx].IsNull {
				count++
			}
		}
		return &types.Value{Type: types.TypeInteger, Data: count, IsNull: false}, nil
	case "SUM":
		var sum float64
		hasValue := false
		for _, row := range rows {
			if colIdx < len(row.Values) && row.Values[colIdx] != nil && !row.Values[colIdx].IsNull {
				val := row.Values[colIdx]
				switch v := val.Data.(type) {
				case int64:
					sum += float64(v)
					hasValue = true
				case float64:
					sum += v
					hasValue = true
				}
			}
		}
		if !hasValue {
			return &types.Value{IsNull: true}, nil
		}
		return &types.Value{Type: types.TypeFloat, Data: sum, IsNull: false}, nil
	case "AVG":
		var sum float64
		count := int64(0)
		for _, row := range rows {
			if colIdx < len(row.Values) && row.Values[colIdx] != nil && !row.Values[colIdx].IsNull {
				val := row.Values[colIdx]
				switch v := val.Data.(type) {
				case int64:
					sum += float64(v)
					count++
				case float64:
					sum += v
					count++
				}
			}
		}
		if count == 0 {
			return &types.Value{IsNull: true}, nil
		}
		return &types.Value{Type: types.TypeFloat, Data: sum / float64(count), IsNull: false}, nil
	default:
		return nil, fmt.Errorf("unsupported aggregate function: %s", funcName)
	}
}

func (e *Executor) executeUpdate(stmt *parser.UpdateStmt) (*Result, error) {
	schema, err := e.storage.GetSchema(stmt.TableName)
	if err != nil {
		return nil, err
	}

	updated := 0

	updatedCount, err := e.storage.Update(stmt.TableName,
		func(row *storage.Row) bool {
			// #region agent log
			func() {
				logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				defer logFile.Close()
				whereResult := true
				if stmt.Where != nil {
					whereResult = e.evaluateExpression(stmt.Where, row, stmt.TableName)
				}
				logData, _ := json.Marshal(map[string]interface{}{
					"sessionId":    "debug-session",
					"runId":        "run1",
					"hypothesisId": "C",
					"location":     "executor.go:245",
					"message":      "Update filter evaluation",
					"data": map[string]interface{}{
						"hasWhere": stmt.Where != nil,
						"result":   whereResult,
						"rowId":    func() interface{} { if len(row.Values) > 0 && row.Values[0] != nil { return row.Values[0].Data }; return nil }(),
					},
					"timestamp": time.Now().UnixMilli(),
				})
				logFile.Write(append(logData, '\n'))
			}()
			// #endregion
			if stmt.Where != nil {
				return e.evaluateExpression(stmt.Where, row, stmt.TableName)
			}
			return true
		},
		func(row *storage.Row) *storage.Row {
			// #region agent log
			func() {
				logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				defer logFile.Close()
				rowId := "unknown"
				if len(row.Values) > 0 && row.Values[0] != nil {
					rowId = fmt.Sprintf("%v", row.Values[0].Data)
				}
				logData, _ := json.Marshal(map[string]interface{}{
					"sessionId":    "debug-session",
					"runId":        "update",
					"hypothesisId": "E",
					"location":     "executor.go:552",
					"message":      "Update function entry",
					"data": map[string]interface{}{
						"rowId":      rowId,
						"setClauses": len(stmt.Set),
					},
					"timestamp": time.Now().UnixMilli(),
				})
				logFile.Write(append(logData, '\n'))
			}()
			// #endregion
			newRow := &storage.Row{Values: make([]*types.Value, len(row.Values))}
			// Deep copy values
			for i, val := range row.Values {
				if val != nil {
					newRow.Values[i] = &types.Value{
						Type:   val.Type,
						Data:   val.Data,
						IsNull: val.IsNull,
					}
				} else {
					newRow.Values[i] = &types.Value{Type: schema.Columns[i].Type, IsNull: true}
				}
			}

			for _, setClause := range stmt.Set {
				// #region agent log
				func() {
					logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					defer logFile.Close()
					setValue := "null"
					if setClause.Value != nil && !setClause.Value.IsNull {
						setValue = setClause.Value.String()
					}
					rowId := "unknown"
					if len(row.Values) > 0 && row.Values[0] != nil {
						rowId = fmt.Sprintf("%v", row.Values[0].Data)
					}
					logData, _ := json.Marshal(map[string]interface{}{
						"sessionId":    "debug-session",
						"runId":        "update",
						"hypothesisId": "E",
						"location":     "executor.go:577",
						"message":      "Applying SET clause",
						"data": map[string]interface{}{
							"rowId":  rowId,
							"column": setClause.Column,
							"value":  setValue,
						},
						"timestamp": time.Now().UnixMilli(),
					})
					logFile.Write(append(logData, '\n'))
				}()
				// #endregion
				for i, col := range schema.Columns {
					if col.Name == setClause.Column {
						if i < len(newRow.Values) {
							// Create a new Value object to avoid sharing references
							if setClause.Value != nil {
								newRow.Values[i] = &types.Value{
									Type:   setClause.Value.Type,
									Data:   setClause.Value.Data,
									IsNull: setClause.Value.IsNull,
								}
							} else {
								newRow.Values[i] = &types.Value{Type: col.Type, IsNull: true}
							}
						}
						break
					}
				}
			}

			// Validate constraints
			if err := e.constraint.ValidateUpdate(stmt.TableName, row, newRow); err != nil {
				return nil
			}

			updated++
			// #region agent log
			if f, err2 := os.OpenFile(".cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err2 == nil {
				rowId := "unknown"
				if len(newRow.Values) > 0 && newRow.Values[0] != nil {
					rowId = fmt.Sprintf("%v", newRow.Values[0].Data)
				}
				updatedValues := make(map[string]string)
				for i, col := range schema.Columns {
					if i < len(newRow.Values) && newRow.Values[i] != nil {
						updatedValues[col.Name] = newRow.Values[i].String()
					}
				}
				json.NewEncoder(f).Encode(map[string]interface{}{"sessionId": "debug-session", "runId": "update", "hypothesisId": "E", "location": "executor.go:572", "message": "Update function exit", "data": map[string]interface{}{"rowId": rowId, "updatedValues": updatedValues}, "timestamp": time.Now().UnixMilli()})
				f.Close()
			}
			// #endregion
			return newRow
		})

	if err != nil {
		return nil, err
	}

	return &Result{Message: fmt.Sprintf("%d row(s) updated", updatedCount)}, nil
}

func (e *Executor) executeDelete(stmt *parser.DeleteStmt) (*Result, error) {
	deleted, err := e.storage.Delete(stmt.TableName,
		func(row *storage.Row) bool {
			if stmt.Where != nil {
				return e.evaluateExpression(stmt.Where, row, stmt.TableName)
			}
			return true
		})

	return &Result{Message: fmt.Sprintf("%d row(s) deleted", deleted)}, err
}

func (e *Executor) evaluateExpression(expr parser.Expression, row *storage.Row, tableName string) bool {
	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "executor.go:341",
			"message":      "evaluateExpression entry",
			"data": map[string]interface{}{
				"exprType": fmt.Sprintf("%T", expr),
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion
	switch ex := expr.(type) {
	case *parser.BinaryExpr:
		return e.evaluateBinary(ex, row, tableName)
	case *parser.LiteralExpr:
		if ex.Value.IsNull {
			return false
		}
		if ex.Value.Type == types.TypeBoolean {
			return ex.Value.Data.(bool)
		}
		return true
	case *parser.ColumnExpr:
		// Column reference - get the value and treat as boolean
		val := e.getExpressionValue(ex, row, tableName)
		if val == nil || val.IsNull {
			return false
		}
		// For boolean types, return the value; for others, return true if not null
		if val.Type == types.TypeBoolean {
			return val.Data.(bool)
		}
		return true
	case *parser.UnaryExpr:
		return !e.evaluateUnary(ex, row, tableName)
	case *parser.InExpr:
		return e.evaluateIn(ex, row, tableName)
	case *parser.IsNullExpr:
		return e.evaluateIsNull(ex, row, tableName)
	default:
		return false
	}
}

func (e *Executor) evaluateBinary(expr *parser.BinaryExpr, row *storage.Row, tableName string) bool {
	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "executor.go:328",
			"message":      "evaluateBinary entry",
			"data": map[string]interface{}{
				"operator": expr.Operator,
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion
	left := e.getExpressionValue(expr.Left, row, tableName)
	right := e.getExpressionValue(expr.Right, row, tableName)

	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "executor.go:333",
			"message":      "evaluateBinary got values",
			"data": map[string]interface{}{
				"operator": expr.Operator,
				"leftNil":  left == nil,
				"rightNil": right == nil,
				"leftData": func() interface{} { if left != nil { return left.Data }; return nil }(),
				"rightData": func() interface{} { if right != nil { return right.Data }; return nil }(),
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion

	if left == nil || right == nil {
		return false
	}

	cmp, err := left.Compare(right)
	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "executor.go:340",
			"message":      "evaluateBinary comparison result",
			"data": map[string]interface{}{
				"operator": expr.Operator,
				"cmp":      cmp,
				"err":      err != nil,
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion
	if err != nil {
		return false
	}

	var result bool
	switch expr.Operator {
	case "=", "==":
		result = cmp == 0
	case "!=", "<>":
		result = cmp != 0
	case "<":
		result = cmp < 0
	case ">":
		result = cmp > 0
	case "<=":
		result = cmp <= 0
	case ">=":
		result = cmp >= 0
	case "AND":
		leftBool := e.evaluateExpression(expr.Left, row, tableName)
		rightBool := e.evaluateExpression(expr.Right, row, tableName)
		result = leftBool && rightBool
	case "OR":
		leftBool := e.evaluateExpression(expr.Left, row, tableName)
		rightBool := e.evaluateExpression(expr.Right, row, tableName)
		result = leftBool || rightBool
	default:
		result = false
	}
	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "executor.go:365",
			"message":      "evaluateBinary result",
			"data": map[string]interface{}{
				"operator": expr.Operator,
				"cmp":      cmp,
				"result":   result,
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion
	return result
}

func (e *Executor) evaluateUnary(expr *parser.UnaryExpr, row *storage.Row, tableName string) bool {
	return e.evaluateExpression(expr.Expr, row, tableName)
}

func (e *Executor) evaluateIn(expr *parser.InExpr, row *storage.Row, tableName string) bool {
	left := e.getExpressionValue(expr.Left, row, tableName)
	if left == nil {
		return false
	}

	for _, rightExpr := range expr.Right {
		right := e.getExpressionValue(rightExpr, row, tableName)
		if right != nil {
			if cmp, _ := left.Compare(right); cmp == 0 {
				return !expr.Not
			}
		}
	}

	return expr.Not
}

func (e *Executor) evaluateIsNull(expr *parser.IsNullExpr, row *storage.Row, tableName string) bool {
	val := e.getExpressionValue(expr.Expr, row, tableName)
	isNull := val == nil || val.IsNull
	if expr.Not {
		return !isNull
	}
	return isNull
}

func (e *Executor) getExpressionValue(expr parser.Expression, row *storage.Row, tableName string) *types.Value {
	switch ex := expr.(type) {
	case *parser.LiteralExpr:
		// #region agent log
		func() {
			logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			defer logFile.Close()
			logData, _ := json.Marshal(map[string]interface{}{
				"sessionId":    "debug-session",
				"runId":        "run1",
				"hypothesisId": "B",
				"location":     "executor.go:483",
				"message":      "getExpressionValue LiteralExpr",
				"data": map[string]interface{}{
					"value":  func() interface{} { if ex.Value != nil { return ex.Value.Data }; return nil }(),
					"isNull": func() bool { if ex.Value != nil { return ex.Value.IsNull }; return true }(),
				},
				"timestamp": time.Now().UnixMilli(),
			})
			logFile.Write(append(logData, '\n'))
		}()
		// #endregion
		return ex.Value
	case *parser.ColumnExpr:
		schema, err := e.storage.GetSchema(tableName)
		if err != nil {
			return nil
		}
		for i, col := range schema.Columns {
			if col.Name == ex.Name {
				if i < len(row.Values) {
					// #region agent log
					func() {
						logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
						defer logFile.Close()
						logData, _ := json.Marshal(map[string]interface{}{
							"sessionId":    "debug-session",
							"runId":        "run1",
							"hypothesisId": "B",
							"location":     "executor.go:498",
							"message":      "getExpressionValue ColumnExpr found",
							"data": map[string]interface{}{
								"colName": col.Name,
								"colIdx":  i,
								"rowVal":  func() interface{} { if row.Values[i] != nil { return row.Values[i].Data }; return nil }(),
								"isNull":  func() bool { if row.Values[i] != nil { return row.Values[i].IsNull }; return true }(),
							},
							"timestamp": time.Now().UnixMilli(),
						})
						logFile.Write(append(logData, '\n'))
					}()
					// #endregion
					return row.Values[i]
				}
			}
		}
		return nil
	case *parser.BinaryExpr:
		// For binary expressions in value context, we'd need to evaluate
		// For now, return nil
		return nil
	default:
		return nil
	}
}

func (e *Executor) shouldSwap(row1, row2 *storage.Row, orderBy []parser.OrderByExpr, schema *storage.TableSchema) bool {
	for _, ob := range orderBy {
		var idx int
		for i, col := range schema.Columns {
			if col.Name == ob.Column {
				idx = i
				break
			}
		}

		if idx >= len(row1.Values) || idx >= len(row2.Values) {
			continue
		}

		val1 := row1.Values[idx]
		val2 := row2.Values[idx]

		if val1 == nil || val1.IsNull {
			return false
		}
		if val2 == nil || val2.IsNull {
			return true
		}

		cmp, _ := val1.Compare(val2)
		if cmp != 0 {
			if ob.Desc {
				return cmp < 0
			}
			return cmp > 0
		}
	}
	return false
}

func (e *Executor) getColumnNames(stmt *parser.SelectStmt, schema *storage.TableSchema) []string {
	if len(stmt.Columns) == 1 && stmt.Columns[0].IsStar {
		names := make([]string, len(schema.Columns))
		for i, col := range schema.Columns {
			names[i] = col.Name
		}
		return names
	}

	names := make([]string, len(stmt.Columns))
	for i, col := range stmt.Columns {
		if col.Alias != "" {
			names[i] = col.Alias
		} else if col.Function != "" {
			// Aggregate function: format as "MAX(id)" or "MAX(id)" with alias
			if col.ArgColumn == "*" {
				names[i] = fmt.Sprintf("%s(*)", col.Function)
			} else {
				names[i] = fmt.Sprintf("%s(%s)", col.Function, col.ArgColumn)
			}
		} else {
			names[i] = col.Name
		}
	}
	return names
}

// Result represents the result of a query execution
type Result struct {
	Rows    []*storage.Row
	Columns []string
	Message string
}
