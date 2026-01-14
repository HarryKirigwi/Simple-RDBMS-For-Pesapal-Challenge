package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"rdbms/internal/types"
	"strconv"
	"strings"
	"time"
)

// Parser parses SQL statements
type Parser struct {
	l      *Lexer
	errors []error
	curToken Token
	peekToken Token
}

// NewParser creates a new parser
func NewParser(input string) *Parser {
	p := &Parser{
		l:      NewLexer(input),
		errors: []error{},
	}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) curTokenIs(t TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) peekError(t TokenType) {
	msg := fmt.Sprintf("expected next token to be %v, got %v instead", t, p.peekToken.Type)
	p.errors = append(p.errors, &Error{Message: msg, Pos: p.peekToken.Pos})
}

// ParseStatement parses a SQL statement
func (p *Parser) ParseStatement() (Statement, error) {
	switch strings.ToUpper(p.curToken.Literal) {
	case "CREATE":
		return p.parseCreateTable()
	case "INSERT":
		return p.parseInsert()
	case "SELECT":
		return p.parseSelect()
	case "UPDATE":
		return p.parseUpdate()
	case "DELETE":
		return p.parseDelete()
	default:
		return nil, fmt.Errorf("unexpected token: %s", p.curToken.Literal)
	}
}

func (p *Parser) parseCreateTable() (*CreateTableStmt, error) {
	stmt := &CreateTableStmt{}

	if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "TABLE" {
		return nil, fmt.Errorf("expected TABLE after CREATE")
	}

	if !p.expectPeek(TokenIdentifier) {
		return nil, fmt.Errorf("expected table name")
	}
	stmt.TableName = p.curToken.Literal

	if !p.expectPeek(TokenPunctuation) || p.curToken.Literal != "(" {
		return nil, fmt.Errorf("expected '(' after table name")
	}

	// Parse columns
	columns := []ColumnDef{}
	primaryKeys := []string{}
	uniqueKeys := [][]string{}

	for !p.curTokenIs(TokenPunctuation) || p.curToken.Literal != ")" {
		p.nextToken()

		if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == ")" {
			break
		}

		if strings.ToUpper(p.curToken.Literal) == "PRIMARY" {
			p.nextToken()
			if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "KEY" {
				return nil, fmt.Errorf("expected KEY after PRIMARY")
			}
			if !p.expectPeek(TokenPunctuation) || p.curToken.Literal != "(" {
				return nil, fmt.Errorf("expected '(' after PRIMARY KEY")
			}
			p.nextToken()
			primaryKeys = append(primaryKeys, p.curToken.Literal)
			if !p.expectPeek(TokenPunctuation) || p.curToken.Literal != ")" {
				return nil, fmt.Errorf("expected ')' after primary key column")
			}
			p.nextToken()
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "," {
				continue
			}
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == ")" {
				break
			}
			continue
		}

		if strings.ToUpper(p.curToken.Literal) == "UNIQUE" {
			p.nextToken()
			if !p.expectPeek(TokenPunctuation) || p.curToken.Literal != "(" {
				return nil, fmt.Errorf("expected '(' after UNIQUE")
			}
			uniqueCols := []string{}
			p.nextToken()
			for !p.curTokenIs(TokenPunctuation) || p.curToken.Literal != ")" {
				uniqueCols = append(uniqueCols, p.curToken.Literal)
				p.nextToken()
				if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "," {
					p.nextToken()
				}
			}
			uniqueKeys = append(uniqueKeys, uniqueCols)
			p.nextToken()
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "," {
				continue
			}
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == ")" {
				break
			}
			continue
		}

		col := ColumnDef{Name: p.curToken.Literal, Nullable: true}

		p.nextToken()
		dataTypeStr := p.curToken.Literal
		if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "(" {
			p.nextToken() // consume '('
			p.nextToken()
			size, err := strconv.Atoi(p.curToken.Literal)
			if err != nil {
				return nil, fmt.Errorf("invalid size for VARCHAR: %w", err)
			}
			col.Size = size
			dataTypeStr += "(" + p.curToken.Literal + ")"
			p.nextToken() // consume ')'
			p.nextToken()
		} else {
			p.nextToken()
		}

		dt, size, err := types.ParseDataType(dataTypeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid data type: %w", err)
		}
		col.Type = dt
		if col.Size == 0 {
			col.Size = size
		}

		// Check for constraints
		for p.peekTokenIs(TokenKeyword) {
			p.nextToken()
			keyword := strings.ToUpper(p.curToken.Literal)
			if keyword == "PRIMARY" {
				p.nextToken()
				if strings.ToUpper(p.curToken.Literal) == "KEY" {
					col.PrimaryKey = true
					primaryKeys = append(primaryKeys, col.Name)
				}
			} else if keyword == "UNIQUE" {
				col.Unique = true
				uniqueKeys = append(uniqueKeys, []string{col.Name})
			} else if keyword == "NOT" {
				p.nextToken()
				if strings.ToUpper(p.curToken.Literal) == "NULL" {
					col.Nullable = false
				}
			}
		}

		columns = append(columns, col)

		if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "," {
			p.nextToken()
		}
	}

	stmt.Columns = columns
	stmt.PrimaryKey = primaryKeys
	stmt.UniqueKeys = uniqueKeys

	return stmt, nil
}

func (p *Parser) parseInsert() (*InsertStmt, error) {
	stmt := &InsertStmt{}

	if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "INTO" {
		return nil, fmt.Errorf("expected INTO after INSERT")
	}

	if !p.expectPeek(TokenIdentifier) {
		return nil, fmt.Errorf("expected table name")
	}
	stmt.TableName = p.curToken.Literal

	// Optional column list
	if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "(" {
		p.nextToken() // consume '('
		p.nextToken()
		columns := []string{}
		for !p.curTokenIs(TokenPunctuation) || p.curToken.Literal != ")" {
			columns = append(columns, p.curToken.Literal)
			p.nextToken()
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "," {
				p.nextToken()
			}
		}
		stmt.Columns = columns
		p.nextToken() // consume ')'
	}

	if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "VALUES" {
		return nil, fmt.Errorf("expected VALUES")
	}

	// Parse value lists
	valueLists := [][]*types.Value{}
	for {
		if !p.expectPeek(TokenPunctuation) || p.curToken.Literal != "(" {
			break
		}
		p.nextToken()

		values := []*types.Value{}
		for {
			// Check if we've reached the closing parenthesis
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == ")" {
				break
			}
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			values = append(values, val)
			// Check if there's a comma (next value) or closing parenthesis
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == ")" {
				break
			}
			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "," {
				p.nextToken() // consume comma, move to next value
			} else {
				// No comma and not closing paren - might be end of values
				break
			}
		}
		valueLists = append(valueLists, values)
		p.nextToken() // consume ')'

		if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "," {
			p.nextToken()
		} else {
			break
		}
	}

	stmt.Values = valueLists
	return stmt, nil
}

func (p *Parser) parseValue() (*types.Value, error) {
	if p.curTokenIs(TokenKeyword) && strings.ToUpper(p.curToken.Literal) == "NULL" {
		p.nextToken()
		return &types.Value{IsNull: true}, nil
	}

	if p.curTokenIs(TokenString) {
		val, err := types.NewValue(types.TypeText, p.curToken.Literal)
		if err != nil {
			return nil, err
		}
		p.nextToken()
		return val, nil
	}

	if p.curTokenIs(TokenInteger) {
		i, _ := strconv.ParseInt(p.curToken.Literal, 10, 64)
		val, _ := types.NewValue(types.TypeInteger, i)
		p.nextToken()
		return val, nil
	}

	if p.curTokenIs(TokenFloat) {
		f, _ := strconv.ParseFloat(p.curToken.Literal, 64)
		val, _ := types.NewValue(types.TypeFloat, f)
		p.nextToken()
		return val, nil
	}

	if p.curTokenIs(TokenKeyword) {
		keyword := strings.ToUpper(p.curToken.Literal)
		if keyword == "TRUE" {
			val, _ := types.NewValue(types.TypeBoolean, true)
			p.nextToken()
			return val, nil
		}
		if keyword == "FALSE" {
			val, _ := types.NewValue(types.TypeBoolean, false)
			p.nextToken()
			return val, nil
		}
	}

	return nil, fmt.Errorf("unexpected token in value: %s", p.curToken.Literal)
}

func (p *Parser) parseSelect() (*SelectStmt, error) {
	stmt := &SelectStmt{}

	// Parse columns
	p.nextToken()
	columns := []SelectColumn{}

	if p.curTokenIs(TokenOperator) && p.curToken.Literal == "*" {
		columns = append(columns, SelectColumn{IsStar: true})
		p.nextToken()
	} else {
		for {
			// #region agent log
			func() {
				logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				defer logFile.Close()
				logData, _ := json.Marshal(map[string]interface{}{
					"sessionId":    "debug-session",
					"runId":        "run1",
					"hypothesisId": "A",
					"location":     "parser.go:350",
					"message":      "parseSelect parsing column",
					"data": map[string]interface{}{
						"curTokenType":  p.curToken.Type,
						"curTokenLit":    p.curToken.Literal,
						"peekTokenType":  p.peekToken.Type,
						"peekTokenLit":   p.peekToken.Literal,
					},
					"timestamp": time.Now().UnixMilli(),
				})
				logFile.Write(append(logData, '\n'))
			}()
			// #endregion
			col := SelectColumn{Name: p.curToken.Literal}
			
			// Handle qualified column names (table.column)
			if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "." {
				p.nextToken() // advance from table name to "."
				p.nextToken() // advance from "." to column name
				col.Name = col.Name + "." + p.curToken.Literal // form "table.column"
				p.nextToken() // advance past column name
			} else {
				p.nextToken() // advance past simple column name
			}
			
			// Check for AS alias
			if p.curTokenIs(TokenKeyword) && strings.ToUpper(p.curToken.Literal) == "AS" {
				p.nextToken() // consume AS
				if p.curTokenIs(TokenIdentifier) {
					col.Alias = p.curToken.Literal
					p.nextToken() // advance past alias
				}
			}
			
			columns = append(columns, col)

			if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "," {
				p.nextToken()
			} else {
				break
			}
		}
	}
	stmt.Columns = columns

	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "B",
			"location":     "parser.go:400",
			"message":      "parseSelect after column parsing",
			"data": map[string]interface{}{
				"curTokenType":  p.curToken.Type,
				"curTokenLit":    p.curToken.Literal,
				"peekTokenType":  p.peekToken.Type,
				"peekTokenLit":   p.peekToken.Literal,
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion

	// After consuming * or columns, curToken should be FROM
	// Check if we're already on FROM, otherwise advance to it
	if !p.curTokenIs(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "FROM" {
		// Not on FROM yet, try to advance
		if !p.expectPeek(TokenKeyword) {
			return nil, fmt.Errorf("expected FROM")
		}
		// After expectPeek, curToken should now be FROM
		if strings.ToUpper(p.curToken.Literal) != "FROM" {
			return nil, fmt.Errorf("expected FROM")
		}
	}
	
	// Now curToken is FROM, advance past it to get the table name
	p.nextToken()

	if !p.curTokenIs(TokenIdentifier) {
		return nil, fmt.Errorf("expected table name")
	}
	stmt.From = p.curToken.Literal

	// Parse JOINs
	for p.peekTokenIs(TokenKeyword) {
		keyword := strings.ToUpper(p.peekToken.Literal)
		if keyword == "INNER" || keyword == "LEFT" || keyword == "RIGHT" || keyword == "FULL" {
			join, err := p.parseJoin()
			if err != nil {
				return nil, err
			}
			stmt.Joins = append(stmt.Joins, join)
		} else {
			break
		}
	}

	// Parse WHERE
	if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "WHERE" {
		p.nextToken()
		p.nextToken()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	}

	// Parse ORDER BY
	if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "ORDER" {
		p.nextToken()
		if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "BY" {
			return nil, fmt.Errorf("expected BY after ORDER")
		}
		orderBy := []OrderByExpr{}
		for {
			p.nextToken()
			expr := OrderByExpr{Column: p.curToken.Literal}
			if p.peekTokenIs(TokenKeyword) {
				p.nextToken()
				if strings.ToUpper(p.curToken.Literal) == "DESC" {
					expr.Desc = true
				}
			}
			orderBy = append(orderBy, expr)
			if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "," {
				p.nextToken()
			} else {
				break
			}
		}
		stmt.OrderBy = orderBy
	}

	// Parse LIMIT
	if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "LIMIT" {
		p.nextToken()
		p.nextToken()
		limit, err := strconv.Atoi(p.curToken.Literal)
		if err != nil {
			return nil, fmt.Errorf("invalid LIMIT value: %w", err)
		}
		stmt.Limit = &limit
	}

	return stmt, nil
}

func (p *Parser) parseJoin() (Join, error) {
	join := Join{}

	p.nextToken()
	keyword := strings.ToUpper(p.curToken.Literal)
	if keyword == "INNER" {
		join.Type = JoinInner
		if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "JOIN" {
			return join, fmt.Errorf("expected JOIN after INNER")
		}
	} else if keyword == "LEFT" {
		join.Type = JoinLeft
		if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "JOIN" {
			return join, fmt.Errorf("expected JOIN after LEFT")
		}
	} else if keyword == "RIGHT" {
		join.Type = JoinRight
		if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "JOIN" {
			return join, fmt.Errorf("expected JOIN after RIGHT")
		}
	} else if keyword == "FULL" {
		join.Type = JoinFull
		if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "OUTER" {
			return join, fmt.Errorf("expected OUTER after FULL")
		}
		if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "JOIN" {
			return join, fmt.Errorf("expected JOIN after FULL OUTER")
		}
	}

	if !p.expectPeek(TokenIdentifier) {
		return join, fmt.Errorf("expected table name in JOIN")
	}
	join.Table = p.curToken.Literal

	if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "ON" {
		return join, fmt.Errorf("expected ON in JOIN")
	}

	p.nextToken()
	expr, err := p.parseExpression()
	if err != nil {
		return join, err
	}
	join.Condition = expr

	return join, nil
}

func (p *Parser) parseUpdate() (*UpdateStmt, error) {
	stmt := &UpdateStmt{}

	if !p.expectPeek(TokenIdentifier) {
		return nil, fmt.Errorf("expected table name")
	}
	stmt.TableName = p.curToken.Literal

	if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "SET" {
		return nil, fmt.Errorf("expected SET")
	}

	setClauses := []SetClause{}
	for {
		p.nextToken()
		clause := SetClause{Column: p.curToken.Literal}
		if !p.expectPeek(TokenOperator) || p.curToken.Literal != "=" {
			return nil, fmt.Errorf("expected '=' in SET clause")
		}
		p.nextToken()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		clause.Value = val
		setClauses = append(setClauses, clause)

		if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "," {
			p.nextToken()
		} else {
			break
		}
	}
	stmt.Set = setClauses

	// #region agent log
	func() {
		logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer logFile.Close()
		hasWhereCur := p.curTokenIs(TokenKeyword) && strings.ToUpper(p.curToken.Literal) == "WHERE"
		hasWherePeek := p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "WHERE"
		logData, _ := json.Marshal(map[string]interface{}{
			"sessionId":    "debug-session",
			"runId":        "run1",
			"hypothesisId": "C",
			"location":     "parser.go:536",
			"message":      "parseUpdate checking for WHERE",
			"data": map[string]interface{}{
				"curTokenType":  p.curToken.Type,
				"curTokenLit":   p.curToken.Literal,
				"peekTokenType": p.peekToken.Type,
				"peekTokenLit":  p.peekToken.Literal,
				"hasWhereCur":   hasWhereCur,
				"hasWherePeek":  hasWherePeek,
			},
			"timestamp": time.Now().UnixMilli(),
		})
		logFile.Write(append(logData, '\n'))
	}()
	// #endregion

	// Check curToken first (parseValue() advances past the value, so curToken might be on WHERE)
	if p.curTokenIs(TokenKeyword) && strings.ToUpper(p.curToken.Literal) == "WHERE" {
		p.nextToken() // Advance past WHERE
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	} else if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "WHERE" {
		p.nextToken() // Advance to WHERE
		p.nextToken() // Advance past WHERE
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
		// #region agent log
		func() {
			logFile, _ := os.OpenFile("c:\\Users\\Administrator\\Code\\RDBMS\\.cursor\\debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			defer logFile.Close()
			logData, _ := json.Marshal(map[string]interface{}{
				"sessionId":    "debug-session",
				"runId":        "run1",
				"hypothesisId": "C",
				"location":     "parser.go:550",
				"message":      "parseUpdate WHERE parsed",
				"data": map[string]interface{}{
					"whereType": fmt.Sprintf("%T", expr),
				},
				"timestamp": time.Now().UnixMilli(),
			})
			logFile.Write(append(logData, '\n'))
		}()
		// #endregion
	}

	return stmt, nil
}

func (p *Parser) parseDelete() (*DeleteStmt, error) {
	stmt := &DeleteStmt{}

	if !p.expectPeek(TokenKeyword) || strings.ToUpper(p.curToken.Literal) != "FROM" {
		return nil, fmt.Errorf("expected FROM after DELETE")
	}

	if !p.expectPeek(TokenIdentifier) {
		return nil, fmt.Errorf("expected table name")
	}
	stmt.TableName = p.curToken.Literal

	if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "WHERE" {
		p.nextToken()
		p.nextToken()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	}

	return stmt, nil
}

func (p *Parser) parseExpression() (Expression, error) {
	return p.parseOrExpression()
}

func (p *Parser) parseOrExpression() (Expression, error) {
	left, err := p.parseAndExpression()
	if err != nil {
		return nil, err
	}

	for p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "OR" {
		p.nextToken()
		p.nextToken()
		right, err := p.parseAndExpression()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: "OR", Right: right}
	}

	return left, nil
}

func (p *Parser) parseAndExpression() (Expression, error) {
	left, err := p.parseComparisonExpression()
	if err != nil {
		return nil, err
	}

	for p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "AND" {
		p.nextToken()
		p.nextToken()
		right, err := p.parseComparisonExpression()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: "AND", Right: right}
	}

	return left, nil
}

func (p *Parser) parseComparisonExpression() (Expression, error) {
	left, err := p.parsePrimaryExpression()
	if err != nil {
		return nil, err
	}

	// After parsePrimaryExpression, curToken is on the operator (if present)
	// Check curToken, not peekToken, because parsePrimaryExpression already advanced
	if p.curTokenIs(TokenOperator) {
		op := p.curToken.Literal
		p.nextToken()
		right, err := p.parsePrimaryExpression()
		if err != nil {
			return nil, err
		}
		return &BinaryExpr{Left: left, Operator: op, Right: right}, nil
	}

	if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "IN" {
		p.nextToken()
		p.nextToken()
		if !p.curTokenIs(TokenPunctuation) || p.curToken.Literal != "(" {
			return nil, fmt.Errorf("expected '(' after IN")
		}
		values := []Expression{}
		p.nextToken()
		for !p.curTokenIs(TokenPunctuation) || p.curToken.Literal != ")" {
			expr, err := p.parsePrimaryExpression()
			if err != nil {
				return nil, err
			}
			values = append(values, expr)
			if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "," {
				p.nextToken()
				p.nextToken()
			} else {
				break
			}
		}
		return &InExpr{Left: left, Right: values, Not: false}, nil
	}

	if p.peekTokenIs(TokenKeyword) && strings.ToUpper(p.peekToken.Literal) == "IS" {
		p.nextToken()
		p.nextToken()
		not := false
		if strings.ToUpper(p.curToken.Literal) == "NOT" {
			not = true
			p.nextToken()
		}
		if strings.ToUpper(p.curToken.Literal) != "NULL" {
			return nil, fmt.Errorf("expected NULL after IS")
		}
		p.nextToken()
		return &IsNullExpr{Expr: left, Not: not}, nil
	}

	return left, nil
}

func (p *Parser) parsePrimaryExpression() (Expression, error) {
	if p.curTokenIs(TokenPunctuation) && p.curToken.Literal == "(" {
		p.nextToken()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if !p.expectPeek(TokenPunctuation) || p.curToken.Literal != ")" {
			return nil, fmt.Errorf("expected ')'")
		}
		p.nextToken()
		return expr, nil
	}

	if p.curTokenIs(TokenIdentifier) {
		col := &ColumnExpr{Name: p.curToken.Literal}
		// Handle qualified column names (table.column)
		if p.peekTokenIs(TokenPunctuation) && p.peekToken.Literal == "." {
			p.nextToken() // advance from table name to "."
			p.nextToken() // advance from "." to column name
			col.Name = col.Name + "." + p.curToken.Literal // form "table.column"
			p.nextToken() // advance past column name
		} else {
			p.nextToken() // advance past simple column name
		}
		return col, nil
	}

	if p.curTokenIs(TokenString) {
		val, _ := types.NewValue(types.TypeText, p.curToken.Literal)
		p.nextToken()
		return &LiteralExpr{Value: val}, nil
	}

	if p.curTokenIs(TokenInteger) {
		i, _ := strconv.ParseInt(p.curToken.Literal, 10, 64)
		val, _ := types.NewValue(types.TypeInteger, i)
		p.nextToken()
		return &LiteralExpr{Value: val}, nil
	}

	if p.curTokenIs(TokenFloat) {
		f, _ := strconv.ParseFloat(p.curToken.Literal, 64)
		val, _ := types.NewValue(types.TypeFloat, f)
		p.nextToken()
		return &LiteralExpr{Value: val}, nil
	}

	if p.curTokenIs(TokenKeyword) {
		keyword := strings.ToUpper(p.curToken.Literal)
		if keyword == "NULL" {
			p.nextToken()
			return &LiteralExpr{Value: &types.Value{IsNull: true}}, nil
		}
		if keyword == "TRUE" {
			val, _ := types.NewValue(types.TypeBoolean, true)
			p.nextToken()
			return &LiteralExpr{Value: val}, nil
		}
		if keyword == "FALSE" {
			val, _ := types.NewValue(types.TypeBoolean, false)
			p.nextToken()
			return &LiteralExpr{Value: val}, nil
		}
		if keyword == "NOT" {
			p.nextToken()
			expr, err := p.parsePrimaryExpression()
			if err != nil {
				return nil, err
			}
			return &UnaryExpr{Operator: "NOT", Expr: expr}, nil
		}
	}

	return nil, fmt.Errorf("unexpected token in expression: %s", p.curToken.Literal)
}
