package main

import (
	"bufio"
	"fmt"
	"os"
	"rdbms/internal/parser"
	"rdbms/internal/query"
	"rdbms/internal/storage"
	"strings"
)

func main() {
	storageEngine, err := storage.NewJSONStorage("./data")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}
	defer storageEngine.Close()

	executor := query.NewExecutor(storageEngine)

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Simple RDBMS - Type SQL commands or .help for help")

	var currentStmt strings.Builder

	for {
		if currentStmt.Len() == 0 {
			fmt.Print("rdbms> ")
		} else {
			fmt.Print("    -> ")
		}

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Handle meta-commands
		if strings.HasPrefix(line, ".") {
			if err := handleMetaCommand(line, storageEngine); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			currentStmt.Reset()
			continue
		}

		currentStmt.WriteString(line)
		currentStmt.WriteString(" ")

		// Check if statement is complete (ends with semicolon)
		stmt := currentStmt.String()
		if strings.HasSuffix(strings.TrimSpace(stmt), ";") {
			stmt = strings.TrimSuffix(strings.TrimSpace(stmt), ";")
			currentStmt.Reset()

			if err := executeStatement(stmt, executor); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

func executeStatement(sql string, executor *query.Executor) error {
	p := parser.NewParser(sql)
	stmt, err := p.ParseStatement()
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	result, err := executor.Execute(stmt)
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	if result.Message != "" {
		fmt.Println(result.Message)
	} else if len(result.Rows) > 0 {
		// Print header
		fmt.Println(strings.Join(result.Columns, " | "))
		fmt.Println(strings.Repeat("-", len(strings.Join(result.Columns, " | "))))

		// Print rows
		for _, row := range result.Rows {
			values := make([]string, len(row.Values))
			for i, val := range row.Values {
				values[i] = val.String()
			}
			fmt.Println(strings.Join(values, " | "))
		}
		fmt.Printf("\n%d row(s)\n", len(result.Rows))
	} else {
		fmt.Println("0 row(s)")
	}

	return nil
}

func handleMetaCommand(cmd string, storage storage.StorageEngine) error {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case ".help":
		fmt.Println("Meta commands:")
		fmt.Println("  .tables              - List all tables")
		fmt.Println("  .schema <table>      - Show table schema")
		fmt.Println("  .exit, .quit         - Exit the REPL")
		fmt.Println("  .help                - Show this help")
	case ".tables":
		tables, err := storage.ListTables()
		if err != nil {
			return err
		}
		if len(tables) == 0 {
			fmt.Println("No tables")
		} else {
			fmt.Println("Tables:")
			for _, table := range tables {
				fmt.Printf("  - %s\n", table)
			}
		}
	case ".schema":
		if len(parts) < 2 {
			return fmt.Errorf("usage: .schema <table>")
		}
		schema, err := storage.GetSchema(parts[1])
		if err != nil {
			return err
		}
		fmt.Printf("Schema for table %s:\n", schema.Name)
		fmt.Println("Columns:")
		for _, col := range schema.Columns {
			nullable := "NULL"
			if !col.Nullable {
				nullable = "NOT NULL"
			}
			fmt.Printf("  - %s %s (%s)\n", col.Name, col.Type, nullable)
		}
		if len(schema.PrimaryKey) > 0 {
			fmt.Printf("Primary Key: %s\n", strings.Join(schema.PrimaryKey, ", "))
		}
		if len(schema.UniqueKeys) > 0 {
			fmt.Println("Unique Keys:")
			for _, uk := range schema.UniqueKeys {
				fmt.Printf("  - %s\n", strings.Join(uk, ", "))
			}
		}
	case ".exit", ".quit":
		os.Exit(0)
	default:
		return fmt.Errorf("unknown command: %s (type .help for help)", parts[0])
	}

	return nil
}
