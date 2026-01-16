# Simple RDBMS in Go

A simple relational database management system implemented in Go with SQL-like interface, JSON-based storage, indexing, and constraint support. The system supports both a command-line REPL (Read-Eval-Print Loop) and a web application interface.

## Features

- **SQL-like Interface**: Supports CREATE TABLE, INSERT, SELECT, UPDATE, DELETE statements
- **Data Types**: INTEGER, VARCHAR(n), TEXT, BOOLEAN, FLOAT, DATE, TIMESTAMP
- **Constraints**: PRIMARY KEY, UNIQUE, and NOT NULL constraints with automatic enforcement
- **Joins**: Support for INNER, LEFT, RIGHT, and FULL OUTER joins using hash-based algorithms
- **Aggregate Functions**: MAX, MIN, COUNT, SUM, AVG
- **REPL Mode**: Interactive command-line interface
- **Web Application**: Simple web app demonstrating CRUD operations with card-based UI
- **Real-time Data Consistency**: Changes in REPL immediately visible in webapp and vice versa

## Building

```bash
# Build the REPL
go build -o rdbms.exe .

# Build the web application
go build -o webapp.exe ./cmd/webapp
```

## Usage

### REPL Mode

Run the REPL to interact with the database:

```bash
./rdbms.exe
```

Example commands:

```sql
-- Create a table
CREATE TABLE users (id INTEGER PRIMARY KEY, name VARCHAR(50), email VARCHAR(100) UNIQUE);

-- Insert data
INSERT INTO users VALUES (1, 'Alice', 'alice@example.com');
INSERT INTO users VALUES (2, 'Bob', 'bob@example.com');

-- Query data
SELECT * FROM users;
SELECT * FROM users WHERE id > 1;

-- Update data
UPDATE users SET name = 'Alice Smith' WHERE id = 1;

-- Delete data
DELETE FROM users WHERE id = 2;
```

Meta commands:
- `.tables` - List all tables
- `.schema <table>` - Show table schema
- `.help` - Show help
- `.exit` or `.quit` - Exit REPL

### Web Application

Run the web application:

```bash
./webapp.exe
```

Then navigate to `http://localhost:8080` in your browser. The web app demonstrates CRUD operations on a `products` table with a modern card-based UI.

**Web App Endpoints**:
- `GET /` - List all products (card grid)
- `GET /create` - Show create form
- `POST /create` - Create product
- `GET /edit/:id` - Show edit form
- `POST /update/:id` - Update product
- `POST /delete/:id` - Delete product

## Data Storage

The RDBMS uses **JSON-based persistent storage** for human-readable data persistence. Data is stored in the `./data` directory:
- **Table schemas**: `*.schema.json` - JSON files containing table structure (columns, types, constraints)
- **Table data**: `*.json` - JSON files containing array of row objects

Each table is stored as a JSON array of objects, where each object represents a row:
```json
[
  {"id": 1, "name": "Alice", "email": "alice@example.com"},
  {"id": 2, "name": "Bob", "email": "bob@example.com"}
]
```

**File Structure**:
```
data/
├── products.schema.json    # Table schema
├── products.json           # Table data
├── users.schema.json
└── users.json
```

**Schema File Format**:
```json
{
  "name": "products",
  "columns": [
    {"name": "id", "type": "INTEGER", "size": 0, "nullable": false},
    {"name": "name", "type": "VARCHAR", "size": 100, "nullable": true},
    {"name": "price", "type": "FLOAT", "size": 0, "nullable": true}
  ],
  "primaryKey": ["id"],
  "uniqueKeys": []
}
```

This approach:
- ✅ Human-readable and easy to debug
- ✅ Simple to implement and understand
- ✅ Easy to inspect data files directly
- ✅ Demonstrates the RDBMS concept clearly
- ✅ Can be easily converted to/from other formats

**Real-time Consistency**:
- Both REPL and webapp use the same `data/` directory
- Both reload data from disk before SELECT/UPDATE/DELETE operations
- Changes in one interface are immediately visible in the other

## Architecture

The system follows a layered architecture:

```
┌─────────────────────────────────────────┐
│         Interface Layer                 │
│  (REPL / Web Application)               │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│         Query Execution Layer            │
│  (Executor, Join Algorithms)              │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│         Parser Layer                     │
│  (Lexer, Parser, AST)                    │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│         Storage Layer                    │
│  (JSONStorage, Schema Manager)           │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│         Type System                      │
│  (Value, DataType, Conversions)          │
└──────────────────────────────────────────┘
```

### Core Components

#### 1. Parser (`internal/parser/`)

The parser converts SQL text into executable statements through two phases:

**Lexer (`lexer.go`)**:
- Tokenizes SQL input into a stream of tokens
- Character-by-character scanning with lookahead
- Token types: Keywords, Identifiers, Strings, Numbers, Operators, Punctuation

**Parser (`parser.go`)**:
- Builds Abstract Syntax Tree (AST) from tokens using recursive descent parsing
- AST nodes: CreateTableStmt, InsertStmt, SelectStmt, UpdateStmt, DeleteStmt, Expression nodes

**Expression Parsing** (Operator Precedence):
```
Expression → OrExpression
OrExpression → AndExpression (OR AndExpression)*
AndExpression → ComparisonExpression (AND ComparisonExpression)*
ComparisonExpression → PrimaryExpression (Operator PrimaryExpression)?
PrimaryExpression → Literal | Column | (Expression) | Function
```

#### 2. Storage Engine (`internal/storage/`)

**JSONStorage (`json_storage.go`)**:
- Persists data in human-readable JSON format
- On initialization: Loads all schema files and corresponding data files
- On INSERT/UPDATE/DELETE: Modifies in-memory table, then serializes to JSON and writes to disk
- On SELECT: Always reloads table from disk for real-time consistency

**Schema Manager (`schema.go`)**:
- Manages table schemas independently from data
- Stores schema definitions in separate JSON files

#### 3. Query Executor (`internal/query/`)

**Executor (`executor.go`)**:
- Executes parsed SQL statements
- Execution flow:
  - `executeCreateTable()` → `storage.CreateTable()`
  - `executeInsert()` → `constraint.ValidateInsert()` → `storage.Insert()`
  - `executeSelect()` → `storage.Select()` → apply WHERE → apply JOIN → apply ORDER BY → apply LIMIT
  - `executeUpdate()` → `storage.Update()` with filter and update functions
  - `executeDelete()` → `storage.Delete()` with filter function

**SELECT Execution Algorithm**:
```
1. Load base table rows from storage
2. Apply WHERE clause filter (if present):
   - Evaluate expression for each row
   - Keep rows where expression evaluates to true
3. Execute JOINs (if present):
   - For each join, perform join algorithm
   - Combine rows from left and right tables
4. Apply column projection:
   - Select specified columns or all columns
   - Handle aggregate functions (MAX, MIN, COUNT, SUM, AVG)
5. Apply ORDER BY (if present):
   - Sort rows by specified columns
6. Apply LIMIT (if present):
   - Return only first N rows
7. Return Result with columns and rows
```

**Expression Evaluation**:
- Recursive evaluation of expression tree
- Types: BinaryExpr, ColumnExpr, LiteralExpr, UnaryExpr, InExpr, IsNullExpr

#### 4. Join Algorithms (`internal/query/join.go`)

The system implements four join types using hash-based algorithms:

**Hash Join Algorithm**:
```
1. Build hash table from right table:
   - Extract join key from each right row
   - Store rows in hash map: hash[key] = [rows]

2. Probe hash table with left table:
   - Extract join key from each left row
   - Lookup matching right rows in hash table
   - Combine left and right rows
```

**Join Types**:
- **INNER JOIN**: Returns only matching rows from both tables
- **LEFT JOIN**: Returns all left rows, with NULL for non-matching right rows
- **RIGHT JOIN**: Returns all right rows, with NULL for non-matching left rows
- **FULL OUTER JOIN**: Returns all rows from both tables, with NULL for non-matching rows

**Time Complexity**: O(n + m + k) - much better than nested loop O(n*m)

#### 5. Constraint Manager (`internal/constraint/`)

**Constraint Types**:
1. **PRIMARY KEY**: Uniqueness + NOT NULL
2. **UNIQUE**: Uniqueness (allows NULL)
3. **NOT NULL**: Column cannot be NULL

**Validation Algorithm**:
- Checks PRIMARY KEY: Extract values, scan existing rows for duplicates or NULL
- Checks UNIQUE constraints: For each unique key set, scan for duplicates
- Checks NOT NULL: Verify non-nullable columns are not NULL

**Time Complexity**: O(n) where n = number of rows (full table scan)

#### 6. Type System (`internal/types/`)

**Data Types**:
- `TypeInteger`: 64-bit signed integer
- `TypeVarchar(n)`: Variable-length string with max length
- `TypeText`: Unlimited length string
- `TypeBoolean`: true/false
- `TypeFloat`: 64-bit floating point
- `TypeDate`: Date value
- `TypeTimestamp`: Date and time value

**Value Representation**:
```go
type Value struct {
    Type   DataType
    Data   interface{}  // Actual value (int64, string, bool, etc.)
    IsNull bool         // NULL indicator
}
```

**Type Conversion**:
- Automatic conversion during INSERT/UPDATE
- JSON deserialization: Numbers unmarshal as `float64`, converted to `int64` for INTEGER
- String parsing: Attempts to parse strings to target type
- Compatibility: VARCHAR and TEXT are compatible types

### Data Flow

**INSERT Operation Flow**:
```
1. User: "INSERT INTO users VALUES (1, 'Alice', 'alice@example.com');"
2. Lexer: Tokenize input → [INSERT, INTO, IDENTIFIER(users), VALUES, ...]
3. Parser: Build InsertStmt AST
4. Executor.executeInsert():
   a. Get table schema
   b. Convert values to correct types
   c. ConstraintManager.ValidateInsert() → check constraints
   d. Storage.Insert() → add row to in-memory table
   e. Storage.saveTable() → serialize to JSON and write to disk
5. Return success message
```

**SELECT Operation Flow**:
```
1. User: "SELECT * FROM users WHERE id > 1;"
2. Lexer: Tokenize input
3. Parser: Build SelectStmt AST with:
   - Columns: [*]
   - From: "users"
   - Where: BinaryExpr(id > 1)
4. Executor.executeSelect():
   a. Storage.Select("users", nil) → load all rows from disk
   b. Apply WHERE filter:
      - For each row: evaluateExpression(whereExpr, row)
      - Keep rows where expression is true
   c. Apply column projection (select all columns)
   d. Return Result
5. Display results
```

**JOIN Operation Flow**:
```
1. User: "SELECT u.name, o.amount FROM users u INNER JOIN orders o ON u.id = o.user_id;"
2. Parser: Build SelectStmt with Join
3. Executor.executeSelect():
   a. Load users table → leftRows
   b. Apply WHERE (if any) to leftRows
   c. executeJoins():
      - Load orders table → rightRows
      - performJoin(leftRows, rightRows, joinCondition):
        * Extract join columns (u.id, o.user_id)
        * Build hash table from rightRows
        * Probe with leftRows
        * Combine matching rows
   d. Apply column projection
   e. Return Result
```

### Query Processing Pipeline

```
SQL Text
  ↓
Lexer (Tokenization)
  ↓
Parser (AST Construction)
  ↓
Executor (Statement Execution)
  ↓
Storage Engine (Data Access)
  ↓
Constraint Manager (Validation)
  ↓
Result
```

### Algorithm Complexity

**Time Complexities**:
- **INSERT**: O(n) - constraint validation requires full table scan
- **SELECT (no WHERE)**: O(n) - load all rows
- **SELECT (with WHERE)**: O(n) - scan all rows, filter
- **SELECT (with JOIN)**: O(n + m + k) - hash join algorithm
- **UPDATE**: O(n) - scan all rows, apply filter
- **DELETE**: O(n) - scan all rows, apply filter

**Space Complexities**:
- **Storage**: O(n) - all rows in memory
- **Hash Join**: O(m) - hash table for right table
- **AST**: O(1) - constant size per statement

## Example: Joins

```sql
-- Create two tables
CREATE TABLE users (id INTEGER PRIMARY KEY, name VARCHAR(50));
CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, amount FLOAT);

-- Insert data
INSERT INTO users VALUES (1, 'Alice');
INSERT INTO users VALUES (2, 'Bob');
INSERT INTO orders VALUES (1, 1, 99.99);
INSERT INTO orders VALUES (2, 1, 49.99);

-- Inner join
SELECT users.name, orders.amount 
FROM users 
INNER JOIN orders ON users.id = orders.user_id;

-- Left join
SELECT users.name, orders.amount 
FROM users 
LEFT JOIN orders ON users.id = orders.user_id;
```

## Limitations

This is a simple educational implementation with the following limitations:
- Single-threaded (no concurrency control)
- Basic query optimization (no cost-based optimizer)
- Full table scans for constraint validation (O(n) complexity)
- No transaction support
- No prepared statements
- No query result caching

## Future Enhancements

1. **Indexing**: B-tree indexes for faster lookups
2. **Query Optimization**: Cost-based query planner
3. **Transactions**: ACID compliance with transaction log
4. **Concurrency**: Multi-user support with locking
5. **Query Caching**: Cache frequently executed queries
6. **Prepared Statements**: SQL injection prevention
7. **Views**: Virtual tables
8. **Triggers**: Automatic actions on data changes
