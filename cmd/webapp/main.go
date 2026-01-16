package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"rdbms/internal/parser"
	"rdbms/internal/query"
	"rdbms/internal/storage"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var executor *query.Executor
var storageEngine storage.StorageEngine

func main() {
	var err error
	storageEngine, err = storage.NewJSONStorage("./data")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storageEngine.Close()

	executor = query.NewExecutor(storageEngine)

	// Initialize demo table if it doesn't exist
	initDemoTable()

	http.HandleFunc("/", listHandler)
	http.HandleFunc("/create", createHandler)
	http.HandleFunc("/edit/", editHandler)
	http.HandleFunc("/update/", updateHandler)
	http.HandleFunc("/delete/", deleteHandler)
	http.HandleFunc("/static/", staticHandler)

	// Create HTTP server
	srv := &http.Server{
		Addr: ":8080",
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		fmt.Println("Web server starting on http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-sigChan
	fmt.Println("\nShutting down gracefully...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Close storage (defer will also call it, but explicit is better)
	if err := storageEngine.Close(); err != nil {
		log.Printf("Error closing storage: %v", err)
	}

	fmt.Println("Server stopped")
}

func initDemoTable() {
	tables, _ := storageEngine.ListTables()
	tableExists := false
	for _, table := range tables {
		if table == "products" {
			tableExists = true
			break
		}
	}

	if !tableExists {
		sql := "CREATE TABLE products (id INTEGER PRIMARY KEY, name VARCHAR(100), price FLOAT, description TEXT);"
		p := parser.NewParser(sql)
		stmt, err := p.ParseStatement()
		if err == nil {
			executor.Execute(stmt)
		}

		// Insert some sample data
		sampleData := []string{
			"INSERT INTO products VALUES (1, 'Laptop', 999.99, 'High-performance laptop');",
			"INSERT INTO products VALUES (2, 'Mouse', 29.99, 'Wireless mouse');",
			"INSERT INTO products VALUES (3, 'Keyboard', 79.99, 'Mechanical keyboard');",
		}
		for _, sql := range sampleData {
			p := parser.NewParser(sql)
			stmt, err := p.ParseStatement()
			if err == nil {
				executor.Execute(stmt)
			}
		}
	}
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sql := "SELECT * FROM products ORDER BY id;"
	p := parser.NewParser(sql)
	stmt, err := p.ParseStatement()
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}

	result, err := executor.Execute(stmt)
	if err != nil {
		http.Error(w, fmt.Sprintf("Execution error: %v", err), http.StatusInternalServerError)
		return
	}

	html := generateListHTML(result)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		html := generateCreateFormHTML()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		name := r.FormValue("name")
		priceStr := r.FormValue("price")
		description := r.FormValue("description")

		// Get next ID
		sql := "SELECT MAX(id) FROM products;"
		p := parser.NewParser(sql)
		stmt, _ := p.ParseStatement()
		result, _ := executor.Execute(stmt)
		nextID := 1
		if len(result.Rows) > 0 && len(result.Rows[0].Values) > 0 && !result.Rows[0].Values[0].IsNull {
			if id, ok := result.Rows[0].Values[0].Data.(int64); ok {
				nextID = int(id) + 1
			}
		}

		// Insert new product
		sql = fmt.Sprintf("INSERT INTO products VALUES (%d, '%s', %s, '%s');",
			nextID, escapeSQL(name), priceStr, escapeSQL(description))
		p = parser.NewParser(sql)
		stmt, err := p.ParseStatement()
		if err != nil {
			http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
			return
		}

		_, err = executor.Execute(stmt)
		if err != nil {
			http.Error(w, fmt.Sprintf("Insert error: %v", err), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func editHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/edit/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sql := fmt.Sprintf("SELECT * FROM products WHERE id = %d;", id)
	p := parser.NewParser(sql)
	stmt, err := p.ParseStatement()
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}

	result, err := executor.Execute(stmt)
	if err != nil || len(result.Rows) == 0 {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	row := result.Rows[0]
	html := generateEditFormHTML(id, row)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/update/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	r.ParseForm()
	name := r.FormValue("name")
	priceStr := r.FormValue("price")
	description := r.FormValue("description")

	sql := fmt.Sprintf("UPDATE products SET name = '%s', price = %s, description = '%s' WHERE id = %d;",
		escapeSQL(name), priceStr, escapeSQL(description), id)
	p := parser.NewParser(sql)
	stmt, err := p.ParseStatement()
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = executor.Execute(stmt)
	if err != nil {
		http.Error(w, fmt.Sprintf("Update error: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/delete/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sql := fmt.Sprintf("DELETE FROM products WHERE id = %d;", id)
	p := parser.NewParser(sql)
	stmt, err := p.ParseStatement()
	if err != nil {
		http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = executor.Execute(stmt)
	if err != nil {
		http.Error(w, fmt.Sprintf("Delete error: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "."+r.URL.Path)
}

func escapeSQL(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), "\\", "\\\\")
}

func generateListHTML(result *query.Result) string {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Products - RDBMS Demo</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <h1>Products</h1>
        <a href="/create" class="btn">Create New Product</a>
        <table>
            <thead>
                <tr>`

	for _, col := range result.Columns {
		html += fmt.Sprintf("<th>%s</th>", col)
	}
	html += "<th>Actions</th>"

	html += `</tr>
            </thead>
            <tbody>`

	for _, row := range result.Rows {
		html += "<tr>"
		for _, val := range row.Values {
			html += fmt.Sprintf("<td>%s</td>", val.String())
		}
		id := row.Values[0].String()
		html += fmt.Sprintf(`<td>
                <a href="/edit/%s" class="btn-small">Edit</a>
                <form method="POST" action="/delete/%s" style="display:inline;">
                    <button type="submit" class="btn-small btn-danger" onclick="return confirm('Are you sure?')">Delete</button>
                </form>
            </td>`, id, id)
		html += "</tr>"
	}

	html += `</tbody>
        </table>
    </div>
</body>
</html>`
	return html
}

func generateCreateFormHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Create Product - RDBMS Demo</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <h1>Create New Product</h1>
        <form method="POST" action="/create">
            <div class="form-group">
                <label>Name:</label>
                <input type="text" name="name" required>
            </div>
            <div class="form-group">
                <label>Price:</label>
                <input type="number" step="0.01" name="price" required>
            </div>
            <div class="form-group">
                <label>Description:</label>
                <textarea name="description"></textarea>
            </div>
            <button type="submit" class="btn">Create</button>
            <a href="/" class="btn btn-secondary">Cancel</a>
        </form>
    </div>
</body>
</html>`
}

func generateEditFormHTML(id int, row *storage.Row) string {
	name := ""
	price := ""
	description := ""

	if len(row.Values) > 1 && !row.Values[1].IsNull {
		name = row.Values[1].String()
	}
	if len(row.Values) > 2 && !row.Values[2].IsNull {
		price = row.Values[2].String()
	}
	if len(row.Values) > 3 && !row.Values[3].IsNull {
		description = row.Values[3].String()
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Edit Product - RDBMS Demo</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <h1>Edit Product</h1>
        <form method="POST" action="/update/%d">
            <div class="form-group">
                <label>Name:</label>
                <input type="text" name="name" value="%s" required>
            </div>
            <div class="form-group">
                <label>Price:</label>
                <input type="number" step="0.01" name="price" value="%s" required>
            </div>
            <div class="form-group">
                <label>Description:</label>
                <textarea name="description">%s</textarea>
            </div>
            <button type="submit" class="btn">Update</button>
            <a href="/" class="btn btn-secondary">Cancel</a>
        </form>
    </div>
</body>
</html>`, id, name, price, description)
}
