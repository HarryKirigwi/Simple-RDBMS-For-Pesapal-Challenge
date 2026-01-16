package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	// Use consistent data directory path (same as REPL)
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

	storageEngine, err = storage.NewJSONStorage(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer storageEngine.Close()

	executor = query.NewExecutor(storageEngine)

	// No initialization - use existing data from shared storage

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
		// If table doesn't exist, show empty list instead of error
		if strings.Contains(err.Error(), "table products not found") {
			// Create empty result
			result = &query.Result{
				Columns: []string{"id", "name", "price", "description"},
				Rows:    []*storage.Row{},
			}
		} else {
			http.Error(w, fmt.Sprintf("Execution error: %v", err), http.StatusInternalServerError)
			return
		}
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
		imageUrl := r.FormValue("image_url")

		// Get next ID
		sql := "SELECT MAX(id) FROM products;"
		p := parser.NewParser(sql)
		stmt, err := p.ParseStatement()
		if err != nil {
			http.Error(w, fmt.Sprintf("Parse error: %v", err), http.StatusInternalServerError)
			return
		}
		result, err := executor.Execute(stmt)
		nextID := 1
		if err != nil {
			// If table doesn't exist, start with ID 1
			if !strings.Contains(err.Error(), "table products not found") {
				http.Error(w, fmt.Sprintf("Execution error: %v", err), http.StatusInternalServerError)
				return
			}
		} else if len(result.Rows) > 0 && len(result.Rows[0].Values) > 0 && !result.Rows[0].Values[0].IsNull {
			if id, ok := result.Rows[0].Values[0].Data.(int64); ok {
				nextID = int(id) + 1
			}
		}

		// Create table if it doesn't exist
		tables, _ := storageEngine.ListTables()
		tableExists := false
		for _, table := range tables {
			if table == "products" {
				tableExists = true
				break
			}
		}

		if !tableExists {
			createSQL := "CREATE TABLE products (id INTEGER PRIMARY KEY, name VARCHAR(100), price FLOAT, description TEXT, image_url TEXT);"
			createParser := parser.NewParser(createSQL)
			createStmt, err := createParser.ParseStatement()
			if err == nil {
				_, err = executor.Execute(createStmt)
				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to create table: %v", err), http.StatusInternalServerError)
					return
				}
			}
		}

		// Insert new product
		imageUrlValue := "NULL"
		if imageUrl != "" {
			imageUrlValue = "'" + escapeSQL(imageUrl) + "'"
		}
		sql = fmt.Sprintf("INSERT INTO products VALUES (%d, '%s', %s, '%s', %s);",
			nextID, escapeSQL(name), priceStr, escapeSQL(description), imageUrlValue)
		p = parser.NewParser(sql)
		stmt, err = p.ParseStatement()
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
	imageUrl := r.FormValue("image_url")

	imageUrlValue := "NULL"
	if imageUrl != "" {
		imageUrlValue = "'" + escapeSQL(imageUrl) + "'"
	}
	sql := fmt.Sprintf("UPDATE products SET name = '%s', price = %s, description = '%s', image_url = %s WHERE id = %d;",
		escapeSQL(name), priceStr, escapeSQL(description), imageUrlValue, id)
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
	// Serve static files from webapp/static directory
	wd, err := os.Getwd()
	var staticPath string
	if err == nil {
		// Check if we're in the project root (has go.mod)
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			staticPath = filepath.Join(wd, "webapp", r.URL.Path)
		} else {
			// Fallback to relative path
			staticPath = filepath.Join("webapp", r.URL.Path)
		}
	} else {
		staticPath = filepath.Join("webapp", r.URL.Path)
	}
	http.ServeFile(w, r, staticPath)
}

func escapeSQL(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), "\\", "\\\\")
}

func generateListHTML(result *query.Result) string {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Pesapal Products</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <h1>Pesapal Products</h1>
        <p class="welcome-message">Welcome to Pesapal products</p>
        <a href="/create" class="btn">Create New Product</a>
        <div class="product-grid">`

	if len(result.Rows) == 0 {
		html += `<div class="empty-state">
            <p>No products found.</p>
            <p>Click "Create New Product" to add your first product.</p>
        </div>`
	} else {
		for _, row := range result.Rows {
			id := row.Values[0].String()
			name := ""
			price := ""
			description := ""
			imageUrl := ""

			if len(row.Values) > 1 && !row.Values[1].IsNull {
				name = row.Values[1].String()
			}
			if len(row.Values) > 2 && !row.Values[2].IsNull {
				price = row.Values[2].String()
			}
			if len(row.Values) > 3 && !row.Values[3].IsNull {
				description = row.Values[3].String()
			}
			if len(row.Values) > 4 && !row.Values[4].IsNull {
				imageUrl = row.Values[4].String()
			}

			// Use placeholder if no image URL
			imgSrc := imageUrl
			if imgSrc == "" {
				// Data URI for placeholder image (simple SVG)
				imgSrc = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='300' height='200'%3E%3Crect fill='%23ddd' width='300' height='200'/%3E%3Ctext fill='%23999' font-family='sans-serif' font-size='18' x='50%25' y='50%25' text-anchor='middle' dy='.3em'%3ENo Image%3C/text%3E%3C/svg%3E"
			}

			html += fmt.Sprintf(`<div class="product-card">
                <div class="product-image">
                    <img src="%s" alt="%s" onerror="this.src='data:image/svg+xml,%%3Csvg xmlns=\\'http://www.w3.org/2000/svg\\' width=\\'300\\' height=\\'200\\'%%3E%%3Crect fill=\\'%%23ddd\\' width=\\'300\\' height=\\'200\\'/%%3E%%3Ctext fill=\\'%%23999\\' font-family=\\'sans-serif\\' font-size=\\'18\\' x=\\'50%%25\\' y=\\'50%%25\\' text-anchor=\\'middle\\' dy=\\'.3em\\'%%3ENo Image%%3C/text%%3E%%3C/svg%%3E'">
                </div>
                <div class="product-info">
                    <h3>%s</h3>
                    <p class="price">Ksh%s</p>
                    <p class="description">%s</p>
                </div>
                <div class="product-actions">
                    <a href="/edit/%s" class="btn-edit">Edit</a>
                    <form method="POST" action="/delete/%s" style="display:inline;">
                        <button type="submit" class="btn-delete" onclick="return confirm('Are you sure?')">Delete</button>
                    </form>
                </div>
            </div>`, imgSrc, name, name, price, description, id, id)
		}
	}

	html += `</div>
    </div>
</body>
</html>`
	return html
}

func generateCreateFormHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Create Product - Pesapal Products</title>
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
            <div class="form-group">
                <label>Image URL (optional):</label>
                <input type="url" name="image_url" placeholder="https://example.com/image.jpg">
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
	imageUrl := ""

	if len(row.Values) > 1 && !row.Values[1].IsNull {
		name = row.Values[1].String()
	}
	if len(row.Values) > 2 && !row.Values[2].IsNull {
		price = row.Values[2].String()
	}
	if len(row.Values) > 3 && !row.Values[3].IsNull {
		description = row.Values[3].String()
	}
	if len(row.Values) > 4 && !row.Values[4].IsNull {
		imageUrl = row.Values[4].String()
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Edit Product - Pesapal Products</title>
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
            <div class="form-group">
                <label>Image URL (optional):</label>
                <input type="url" name="image_url" value="%s" placeholder="https://example.com/image.jpg">
            </div>
            <button type="submit" class="btn">Update</button>
            <a href="/" class="btn btn-secondary">Cancel</a>
        </form>
    </div>
</body>
</html>`, id, name, price, description, imageUrl)
}
