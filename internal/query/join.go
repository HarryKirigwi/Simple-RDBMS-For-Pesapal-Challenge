package query

import (
	"encoding/json"
	"fmt"
	"os"
	"rdbms/internal/parser"
	"rdbms/internal/storage"
	"rdbms/internal/types"
	"strings"
	"time"
)

// executeJoins executes all joins and returns the joined rows
func (e *Executor) executeJoins(leftRows []*storage.Row, leftTable string, joins []parser.Join) ([]*storage.Row, error) {
	currentRows := leftRows
	currentTable := leftTable

	for _, join := range joins {
		rightRows, err := e.storage.Select(join.Table, nil)
		if err != nil {
			return nil, err
		}

		joinedRows, err := e.performJoin(currentRows, currentTable, rightRows, join.Table, join)
		if err != nil {
			return nil, err
		}

		currentRows = joinedRows
		// For simplicity, we'll track the left table name
		// In a full implementation, we'd track all joined tables
	}

	return currentRows, nil
}

// performJoin performs a single join operation
func (e *Executor) performJoin(leftRows []*storage.Row, leftTable string, rightRows []*storage.Row, rightTable string, join parser.Join) ([]*storage.Row, error) {
	leftSchema, err := e.storage.GetSchema(leftTable)
	if err != nil {
		return nil, err
	}

	rightSchema, err := e.storage.GetSchema(rightTable)
	if err != nil {
		return nil, err
	}

	// Extract join condition columns
	leftCol, rightCol, err := e.extractJoinColumns(join.Condition, leftSchema, rightSchema)
	if err != nil {
		return nil, err
	}

	var leftColIdx, rightColIdx int
	for i, col := range leftSchema.Columns {
		if col.Name == leftCol {
			leftColIdx = i
			break
		}
	}
	for i, col := range rightSchema.Columns {
		if col.Name == rightCol {
			rightColIdx = i
			break
		}
	}

	// Build hash table for right side (for hash join)
	rightHash := make(map[string][]*storage.Row)
	for _, rightRow := range rightRows {
		if rightColIdx < len(rightRow.Values) {
			key := e.getJoinKey(rightRow.Values[rightColIdx])
			rightHash[key] = append(rightHash[key], rightRow)
		}
	}

	result := []*storage.Row{}

	switch join.Type {
	case parser.JoinInner:
		result = e.innerJoin(leftRows, rightRows, leftColIdx, rightColIdx, rightHash)
	case parser.JoinLeft:
		result = e.leftJoin(leftRows, rightRows, leftColIdx, rightColIdx, rightHash)
	case parser.JoinRight:
		result = e.rightJoin(leftRows, rightRows, leftColIdx, rightColIdx, rightHash)
	case parser.JoinFull:
		result = e.fullJoin(leftRows, rightRows, leftColIdx, rightColIdx, rightHash)
	default:
		return nil, fmt.Errorf("unsupported join type: %d", join.Type)
	}

	return result, nil
}

func (e *Executor) innerJoin(leftRows []*storage.Row, rightRows []*storage.Row, leftColIdx, rightColIdx int, rightHash map[string][]*storage.Row) []*storage.Row {
	result := []*storage.Row{}

	for _, leftRow := range leftRows {
		if leftColIdx >= len(leftRow.Values) {
			continue
		}

		key := e.getJoinKey(leftRow.Values[leftColIdx])
		matchingRightRows := rightHash[key]

		for _, rightRow := range matchingRightRows {
			joinedRow := e.combineRows(leftRow, rightRow)
			result = append(result, joinedRow)
		}
	}

	return result
}

func (e *Executor) leftJoin(leftRows []*storage.Row, rightRows []*storage.Row, leftColIdx, rightColIdx int, rightHash map[string][]*storage.Row) []*storage.Row {
	result := []*storage.Row{}

	for _, leftRow := range leftRows {
		if leftColIdx >= len(leftRow.Values) {
			// Left row with NULL right side
			numCols := 0
			if len(rightRows) > 0 {
				numCols = len(rightRows[0].Values)
			}
			nullRightRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(leftRow, nullRightRow)
			result = append(result, joinedRow)
			continue
		}

		key := e.getJoinKey(leftRow.Values[leftColIdx])
		matchingRightRows := rightHash[key]

		if len(matchingRightRows) == 0 {
			// No match - left row with NULL right side
			numCols := 0
			if len(rightRows) > 0 {
				numCols = len(rightRows[0].Values)
			}
			nullRightRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(leftRow, nullRightRow)
			result = append(result, joinedRow)
		} else {
			for _, rightRow := range matchingRightRows {
				joinedRow := e.combineRows(leftRow, rightRow)
				result = append(result, joinedRow)
			}
		}
	}

	return result
}

func (e *Executor) rightJoin(leftRows []*storage.Row, rightRows []*storage.Row, leftColIdx, rightColIdx int, rightHash map[string][]*storage.Row) []*storage.Row {
	result := []*storage.Row{}

	// Build hash for left side
	leftHash := make(map[string][]*storage.Row)
	for _, leftRow := range leftRows {
		if leftColIdx < len(leftRow.Values) {
			key := e.getJoinKey(leftRow.Values[leftColIdx])
			leftHash[key] = append(leftHash[key], leftRow)
		}
	}

	for _, rightRow := range rightRows {
		if rightColIdx >= len(rightRow.Values) {
			// Right row with NULL left side
			numCols := 0
			if len(leftRows) > 0 {
				numCols = len(leftRows[0].Values)
			}
			nullLeftRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(nullLeftRow, rightRow)
			result = append(result, joinedRow)
			continue
		}

		key := e.getJoinKey(rightRow.Values[rightColIdx])
		matchingLeftRows := leftHash[key]

		if len(matchingLeftRows) == 0 {
			// No match - right row with NULL left side
			numCols := 0
			if len(leftRows) > 0 {
				numCols = len(leftRows[0].Values)
			}
			nullLeftRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(nullLeftRow, rightRow)
			result = append(result, joinedRow)
		} else {
			for _, leftRow := range matchingLeftRows {
				joinedRow := e.combineRows(leftRow, rightRow)
				result = append(result, joinedRow)
			}
		}
	}

	return result
}

func (e *Executor) fullJoin(leftRows []*storage.Row, rightRows []*storage.Row, leftColIdx, rightColIdx int, rightHash map[string][]*storage.Row) []*storage.Row {
	result := []*storage.Row{}

	// Track which right rows have been matched
	rightMatched := make(map[int]bool)

	// Left join part
	for _, leftRow := range leftRows {
		if leftColIdx >= len(leftRow.Values) {
			numCols := 0
			if len(rightRows) > 0 {
				numCols = len(rightRows[0].Values)
			}
			nullRightRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(leftRow, nullRightRow)
			result = append(result, joinedRow)
			continue
		}

		key := e.getJoinKey(leftRow.Values[leftColIdx])
		matchingRightRows := rightHash[key]

		if len(matchingRightRows) == 0 {
			numCols := 0
			if len(rightRows) > 0 {
				numCols = len(rightRows[0].Values)
			}
			nullRightRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(leftRow, nullRightRow)
			result = append(result, joinedRow)
		} else {
			for i, rightRow := range rightRows {
				if rightColIdx < len(rightRow.Values) && e.getJoinKey(rightRow.Values[rightColIdx]) == key {
					rightMatched[i] = true
					joinedRow := e.combineRows(leftRow, rightRow)
					result = append(result, joinedRow)
				}
			}
		}
	}

	// Add unmatched right rows
	for i, rightRow := range rightRows {
		if !rightMatched[i] {
			numCols := 0
			if len(leftRows) > 0 {
				numCols = len(leftRows[0].Values)
			}
			nullLeftRow := e.createNullRow(numCols)
			joinedRow := e.combineRows(nullLeftRow, rightRow)
			result = append(result, joinedRow)
		}
	}

	return result
}

func (e *Executor) extractJoinColumns(condition parser.Expression, leftSchema, rightSchema *storage.TableSchema) (string, string, error) {
	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "join.go:258",
			"message":      "extractJoinColumns entry",
			"data": map[string]interface{}{
				"conditionType": fmt.Sprintf("%T", condition),
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion
	binaryExpr, ok := condition.(*parser.BinaryExpr)
	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "A",
			"location":     "join.go:264",
			"message":      "extractJoinColumns type check",
			"data": map[string]interface{}{
				"isBinaryExpr": ok,
				"operator":     func() string { if ok { return binaryExpr.Operator }; return "" }(),
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion
	if !ok || binaryExpr.Operator != "=" {
		return "", "", fmt.Errorf("join condition must be an equality comparison")
	}

	leftCol, ok := binaryExpr.Left.(*parser.ColumnExpr)
	if !ok {
		return "", "", fmt.Errorf("left side of join condition must be a column")
	}

	rightCol, ok := binaryExpr.Right.(*parser.ColumnExpr)
	if !ok {
		return "", "", fmt.Errorf("right side of join condition must be a column")
	}

	// Handle qualified column names (table.column)
	leftColName := leftCol.Name
	rightColName := rightCol.Name
	
	// Extract column name from qualified names
	if strings.Contains(leftCol.Name, ".") {
		parts := strings.Split(leftCol.Name, ".")
		leftColName = parts[1] // column name part
	}
	if strings.Contains(rightCol.Name, ".") {
		parts := strings.Split(rightCol.Name, ".")
		rightColName = parts[1] // column name part
	}

	// Determine which column belongs to which table
	// Check if column names match schema columns
	var leftColFound, rightColFound bool
	for _, col := range leftSchema.Columns {
		if col.Name == leftColName {
			leftColFound = true
			break
		}
	}
	for _, col := range rightSchema.Columns {
		if col.Name == rightColName {
			rightColFound = true
			break
		}
	}

	// If both found in their respective schemas, use them
	// Otherwise, try to match based on which table they're qualified with
	if leftColFound && rightColFound {
		return leftColName, rightColName, nil
	}

	// If left column is qualified, check which table it refers to
	if strings.Contains(leftCol.Name, ".") {
		// Check if it matches left or right table name (would need table names passed in)
		// For now, assume left table if found in left schema
		if leftColFound {
			return leftColName, rightColName, nil
		}
		// If not in left, must be in right
		return rightColName, leftColName, nil
	}

	// Similar for right column
	if strings.Contains(rightCol.Name, ".") {
		if rightColFound {
			return leftColName, rightColName, nil
		}
		return rightColName, leftColName, nil
	}

	// Default: assume left is in left table, right is in right table
	return leftColName, rightColName, nil
}

func (e *Executor) getJoinKey(val *types.Value) string {
	if val == nil || val.IsNull {
		return "__NULL__"
	}
	return val.String()
}

func (e *Executor) combineRows(left, right *storage.Row) *storage.Row {
	combined := &storage.Row{
		Values: make([]*types.Value, len(left.Values)+len(right.Values)),
	}
	copy(combined.Values, left.Values)
	copy(combined.Values[len(left.Values):], right.Values)
	return combined
}

func (e *Executor) createNullRow(numCols int) *storage.Row {
	row := &storage.Row{Values: make([]*types.Value, numCols)}
	for i := range row.Values {
		row.Values[i] = &types.Value{IsNull: true}
	}
	return row
}
