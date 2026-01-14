# Simple RDBMS in Go

A simple relational database management system implemented in Go with SQL-like interface, file-based storage, indexing, and constraint support.

## Features

- **SQL-like Interface**: Supports CREATE TABLE, INSERT, SELECT, UPDATE, DELETE statements
- **Data Types**: INTEGER, VARCHAR(n), TEXT, BOOLEAN, FLOAT, DATE, TIMESTAMP
- **Constraints**: PRIMARY KEY and UNIQUE constraints with automatic enforcement
- **Indexing**: B-tree indexing for primary keys
- **Joins**: Support for INNER, LEFT, RIGHT, and FULL OUTER joins
- **REPL Mode**: Interactive command-line interface
- **Web Application**: Simple web app demonstrating CRUD operations

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

Then navigate to `http://localhost:8080` in your browser. The web app demonstrates CRUD operations on a `products` table with sample data.

## Architecture

- **Parser**: Recursive descent SQL parser with lexer and AST
- **Storage**: File-based storage engine with B-tree indexing
- **Query Engine**: Executes parsed statements with WHERE filtering and joins
- **Constraints**: Enforces PRIMARY KEY and UNIQUE constraints
- **Types**: Type system with serialization/deserialization

## Data Storage

The RDBMS uses **JSON-based persistent storage** (similar to MongoDB's document storage). Data is stored in the `./data` directory:
- **Table schemas**: `*.schema.json` - JSON files containing table structure (columns, types, constraints)
- **Table data**: `*.json` - JSON files containing array of row objects (human-readable, easy to debug)

Each table is stored as a JSON array of objects, where each object represents a row:
```json
[
  {"id": 1, "name": "Alice", "email": "alice@example.com"},
  {"id": 2, "name": "Bob", "email": "bob@example.com"}
]
```

This approach:
- ✅ Human-readable and easy to debug
- ✅ Simple to implement and understand
- ✅ Easy to inspect data files directly
- ✅ Demonstrates the RDBMS concept clearly
- ✅ Can be easily converted to/from other formats

The JSON storage is automatically converted to relational table format when displaying results in the REPL.

## Limitations

This is a simple educational implementation with the following limitations:
- Single-threaded (no concurrency control)
- Basic query optimization
- Simplified join algorithms
- Limited error handling in some edge cases
- No transaction support
- No prepared statements

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
