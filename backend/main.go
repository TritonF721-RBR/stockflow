package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
)

type Product struct {
	ID             int        `json:"id"`
	SKU            string     `json:"sku"`
	Name           string     `json:"name"`
	Price          int        `json:"price"`
	Stock          int        `json:"stock"`
	IsActive       bool       `json:"is_active"`
	LastMovementAt *time.Time `json:"last_movement_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ProductsResponse struct {
	Data  []Product `json:"data"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Total int       `json:"total"`
}

type StockAdjustmentRequest struct {
	Type string `json:"type"` // "in" atau "out"
	Qty  int    `json:"qty"`
	Note string `json:"note"`
}

type StockMovement struct {
	ID        int       `json:"id"`
	ProductID int       `json:"product_id"`
	Type      string    `json:"type"`
	Qty       int       `json:"qty"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

var db *sql.DB

func main() {
	var err error
	connStr := "host=localhost port=5432 user=postgres password=password dbname=stockflow sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatalf("Gagal terhubung ke database: %v", err)
	}
	fmt.Println("Database PostgreSQL terhubung!")

	r := chi.NewRouter()

	// Endpoint Produk & Stok
	r.Get("/api/products", getProductsHandler)
	r.Post("/api/products/{id}/stock", adjustStockHandler)
	r.Get("/api/products/{id}/movements", getMovementsHandler)

	fmt.Println("Backend server berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func getProductsHandler(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	var total int
	countQuery := `
		SELECT COUNT(*) 
		FROM products 
		WHERE ($1 = '' OR sku ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
	`
	if err := db.QueryRowContext(r.Context(), countQuery, search).Scan(&total); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	query := `
		SELECT 
			p.id, p.sku, p.name, p.price, p.stock, p.is_active, p.created_at,
			(SELECT created_at FROM stock_movements WHERE product_id = p.id ORDER BY created_at DESC LIMIT 1) AS last_movement_at
		FROM products p
		WHERE ($1 = '' OR p.sku ILIKE '%' || $1 || '%' OR p.name ILIKE '%' || $1 || '%')
		ORDER BY p.id ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := db.QueryContext(r.Context(), query, search, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.IsActive, &p.CreatedAt, &p.LastMovementAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		products = append(products, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ProductsResponse{
		Data:  products,
		Page:  page,
		Limit: limit,
		Total: total,
	})
}

func adjustStockHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	productID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	var req StockAdjustmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validasi input
	if req.Qty <= 0 {
		http.Error(w, "Quantity must be greater than 0", http.StatusBadRequest)
		return
	}
	if req.Type != "in" && req.Type != "out" {
		http.Error(w, "Type must be 'in' or 'out'", http.StatusBadRequest)
		return
	}

	// Buka Transaksi Atomik
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Kunci baris produk dengan SELECT ... FOR UPDATE
	var currentStock int
	err = tx.QueryRowContext(r.Context(), "SELECT stock FROM products WHERE id = $1 FOR UPDATE", productID).Scan(&currentStock)
	if err == sql.ErrNoRows {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Perhitungan stok baru & validasi overdraw
	var newStock int
	if req.Type == "in" {
		newStock = currentStock + req.Qty
	} else {
		if currentStock < req.Qty {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict) // 409
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":      "INSUFFICIENT_STOCK",
					"available": currentStock,
				},
			})
			return
		}
		newStock = currentStock - req.Qty
	}

	// Update stok produk
	_, err = tx.ExecContext(r.Context(), "UPDATE products SET stock = $1 WHERE id = $2", newStock, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Tulis log ke tabel stock_movements
	_, err = tx.ExecContext(r.Context(), "INSERT INTO stock_movements (product_id, type, qty, note) VALUES ($1, $2, $3, $4)",
		productID, req.Type, req.Qty, req.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Stock adjusted successfully",
		"new_stock": newStock,
	})
}

func getMovementsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	productID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	rows, err := db.QueryContext(r.Context(), "SELECT id, product_id, type, qty, COALESCE(note, ''), created_at FROM stock_movements WHERE product_id = $1 ORDER BY created_at DESC", productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	movements := []StockMovement{}
	for rows.Next() {
		var m StockMovement
		if err := rows.Scan(&m.ID, &m.ProductID, &m.Type, &m.Qty, &m.Note, &m.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		movements = append(movements, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": movements,
	})
}
