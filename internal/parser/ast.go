package parser

import "rdbms/internal/types"

// Statement represents a SQL statement
type Statement interface {
	stmtNode()
}

// CreateTableStmt represents a CREATE TABLE statement
type CreateTableStmt struct {
	TableName string
	Columns   []ColumnDef
	PrimaryKey []string
	UniqueKeys [][]string
}

func (*CreateTableStmt) stmtNode() {}

// ColumnDef represents a column definition in CREATE TABLE
type ColumnDef struct {
	Name     string
	Type     types.DataType
	Size     int
	Nullable bool
	PrimaryKey bool
	Unique   bool
}

// InsertStmt represents an INSERT INTO statement
type InsertStmt struct {
	TableName string
	Columns   []string // Optional column list
	Values    [][]*types.Value
}

func (*InsertStmt) stmtNode() {}

// SelectStmt represents a SELECT statement
type SelectStmt struct {
	Columns   []SelectColumn
	From      string
	Joins     []Join
	Where     Expression
	OrderBy   []OrderByExpr
	Limit     *int
}

func (*SelectStmt) stmtNode() {}

// SelectColumn represents a column in SELECT
type SelectColumn struct {
	Name      string
	Alias     string
	IsStar    bool // SELECT *
	Function  string // Aggregate function: MAX, MIN, COUNT, SUM, AVG
	ArgColumn string // Column argument for aggregate function
}

// Join represents a JOIN clause
type Join struct {
	Type      JoinType
	Table     string
	Condition Expression
}

// JoinType represents the type of join
type JoinType int

const (
	JoinInner JoinType = iota
	JoinLeft
	JoinRight
	JoinFull
)

// OrderByExpr represents an ORDER BY expression
type OrderByExpr struct {
	Column string
	Desc   bool
}

// UpdateStmt represents an UPDATE statement
type UpdateStmt struct {
	TableName string
	Set       []SetClause
	Where     Expression
}

func (*UpdateStmt) stmtNode() {}

// SetClause represents a SET clause in UPDATE
type SetClause struct {
	Column string
	Value  *types.Value
}

// DeleteStmt represents a DELETE FROM statement
type DeleteStmt struct {
	TableName string
	Where     Expression
}

func (*DeleteStmt) stmtNode() {}

// Expression represents a SQL expression
type Expression interface {
	exprNode()
}

// BinaryExpr represents a binary expression (e.g., a = b, a > b)
type BinaryExpr struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (*BinaryExpr) exprNode() {}

// LiteralExpr represents a literal value
type LiteralExpr struct {
	Value *types.Value
}

func (*LiteralExpr) exprNode() {}

// ColumnExpr represents a column reference
type ColumnExpr struct {
	Name string
}

func (*ColumnExpr) exprNode() {}

// UnaryExpr represents a unary expression (e.g., NOT, -)
type UnaryExpr struct {
	Operator string
	Expr     Expression
}

func (*UnaryExpr) exprNode() {}

// InExpr represents an IN expression
type InExpr struct {
	Left  Expression
	Right []Expression
	Not   bool
}

func (*InExpr) exprNode() {}

// IsNullExpr represents an IS NULL or IS NOT NULL expression
type IsNullExpr struct {
	Expr Expression
	Not  bool
}

func (*IsNullExpr) exprNode() {}
