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

	r.Get("/api/products", getProductsHandler)

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
