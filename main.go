package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"rdbms/internal/parser"
	"rdbms/internal/query"
	"rdbms/internal/storage"
	"strings"
	"syscall"
)

func main() {
	// Use consistent data directory path (same as webapp)
	// Try to use absolute path from current working directory
	wd, err := os.Getwd()
	var dataDir string
	if err == nil {
		// Check if we're in the project root (has go.mod)
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			dataDir = filepath.Join(wd, "data")
		} else {
			// Fallback to relative path
			dataDir = "./data"
		}
	} else {
		dataDir = "./data"
	}

	storageEngine, err := storage.NewJSONStorage(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}
	defer storageEngine.Close()

	executor := query.NewExecutor(storageEngine)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Flag to track if we should exit
	shouldExit := false

	// Goroutine to handle signals
	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
		shouldExit = true
		// Close stdin to break the scanner loop
		os.Stdin.Close()
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Simple RDBMS - Type SQL commands or .help for help")

	var currentStmt strings.Builder

	for {
		if shouldExit {
			break
		}

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
			if err := handleMetaCommand(line, storageEngine, &shouldExit); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			currentStmt.Reset()
			if shouldExit {
				break
			}
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

func handleMetaCommand(cmd string, storage storage.StorageEngine, shouldExit *bool) error {
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
		// Explicitly close storage before exiting
		if err := storage.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing storage: %v\n", err)
		}
		*shouldExit = true
	default:
		return fmt.Errorf("unknown command: %s (type .help for help)", parts[0])
	}

	return nil
}
