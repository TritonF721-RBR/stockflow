package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
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
	Type string `json:"type"`
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

type OrderItemRequest struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

type CreateOrderRequest struct {
	Customer string             `json:"customer"`
	Items    []OrderItemRequest `json:"items"`
}

type OrderItemResponse struct {
	ID        int    `json:"id"`
	SKU       string `json:"sku"`
	Qty       int    `json:"qty"`
	UnitPrice int    `json:"unit_price"`
	Subtotal  int    `json:"subtotal"`
}

type SalesOrderResponse struct {
	ID          int                 `json:"id"`
	Customer    string              `json:"customer"`
	Status      string              `json:"status"`
	Total       int                 `json:"total"`
	CreatedAt   time.Time           `json:"created_at"`
	CancelledAt *time.Time          `json:"cancelled_at"`
	Items       []OrderItemResponse `json:"items,omitempty"`
}

var db *sql.DB

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	var err error
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	connStr := fmt.Sprintf("host=%s port=5432 user=postgres password=password dbname=stockflow sslmode=disable", dbHost)
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
	r.Use(corsMiddleware)

	// Endpoints Produk & Stok
	r.Get("/api/products", getProductsHandler)
	r.Post("/api/products/{id}/stock", adjustStockHandler)
	r.Get("/api/products/{id}/movements", getMovementsHandler)

	// Endpoints Sales Order
	r.Post("/api/orders", createOrderHandler)
	r.Get("/api/orders", getOrdersHandler)
	r.Post("/api/orders/{id}/cancel", cancelOrderHandler)

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

	if req.Qty <= 0 {
		http.Error(w, "Quantity must be greater than 0", http.StatusBadRequest)
		return
	}
	if req.Type != "in" && req.Type != "out" {
		http.Error(w, "Type must be 'in' or 'out'", http.StatusBadRequest)
		return
	}

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var currentStock int
	err = tx.QueryRowContext(r.Context(), "SELECT stock FROM products WHERE id = $1 FOR UPDATE", productID).Scan(&currentStock)
	if err == sql.ErrNoRows {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var newStock int
	if req.Type == "in" {
		newStock = currentStock + req.Qty
	} else {
		if currentStock < req.Qty {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
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

	_, err = tx.ExecContext(r.Context(), "UPDATE products SET stock = $1 WHERE id = $2", newStock, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Customer == "" || len(req.Items) == 0 {
		http.Error(w, "Customer and items are required", http.StatusBadRequest)
		return
	}

	itemMap := make(map[string]int)
	for _, item := range req.Items {
		if item.Qty <= 0 {
			http.Error(w, "Quantity must be greater than 0", http.StatusBadRequest)
			return
		}
		itemMap[item.SKU] += item.Qty
	}

	skus := make([]string, 0, len(itemMap))
	for sku := range itemMap {
		skus = append(skus, sku)
	}
	sort.Strings(skus)

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	type lockedProduct struct {
		id    int
		sku   string
		price int
		stock int
	}

	lockedItems := make(map[string]lockedProduct)
	totalOrder := 0

	for _, sku := range skus {
		reqQty := itemMap[sku]
		var lp lockedProduct

		query := `SELECT id, sku, price, stock, is_active FROM products WHERE sku = $1 FOR UPDATE`
		var isActive bool
		err := tx.QueryRowContext(r.Context(), query, sku).Scan(&lp.id, &lp.sku, &lp.price, &lp.stock, &isActive)
		if err == sql.ErrNoRows {
			http.Error(w, fmt.Sprintf("Product %s not found", sku), http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !isActive {
			http.Error(w, fmt.Sprintf("Product %s is inactive", sku), http.StatusBadRequest)
			return
		}

		if lp.stock < reqQty {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":      "INSUFFICIENT_STOCK",
					"sku":       sku,
					"requested": reqQty,
					"available": lp.stock,
				},
			})
			return
		}

		lockedItems[sku] = lp
		totalOrder += lp.price * reqQty
	}

	var orderID int
	orderQuery := `INSERT INTO sales_orders (customer, status, total) VALUES ($1, 'CREATED', $2) RETURNING id`
	if err := tx.QueryRowContext(r.Context(), orderQuery, req.Customer, totalOrder).Scan(&orderID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respItems := []OrderItemResponse{}
	for _, sku := range skus {
		reqQty := itemMap[sku]
		lp := lockedItems[sku]

		_, err = tx.ExecContext(r.Context(),
			`INSERT INTO sales_order_items (order_id, sku, qty, unit_price) VALUES ($1, $2, $3, $4)`,
			orderID, sku, reqQty, lp.price)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		newStock := lp.stock - reqQty
		_, err = tx.ExecContext(r.Context(), `UPDATE products SET stock = $1 WHERE id = $2`, newStock, lp.id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		orderNote := fmt.Sprintf("Sales Order #%d", orderID)
		_, err = tx.ExecContext(r.Context(),
			`INSERT INTO stock_movements (product_id, type, qty, note) VALUES ($1, 'out', $2, $3)`,
			lp.id, reqQty, orderNote)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		respItems = append(respItems, OrderItemResponse{
			SKU:       sku,
			Qty:       reqQty,
			UnitPrice: lp.price,
			Subtotal:  lp.price * reqQty,
		})
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       orderID,
		"customer": req.Customer,
		"status":   "CREATED",
		"total":    totalOrder,
		"items":    respItems,
	})
}

func getOrdersHandler(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, customer, status, total, created_at, cancelled_at
		FROM sales_orders
		ORDER BY id DESC
	`
	rows, err := db.QueryContext(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	orders := []SalesOrderResponse{}
	for rows.Next() {
		var o SalesOrderResponse
		if err := rows.Scan(&o.ID, &o.Customer, &o.Status, &o.Total, &o.CreatedAt, &o.CancelledAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		orders = append(orders, o)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": orders,
	})
}

func cancelOrderHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	orderID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var currentStatus string
	err = tx.QueryRowContext(r.Context(), "SELECT status FROM sales_orders WHERE id = $1 FOR UPDATE", orderID).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if currentStatus == "CANCELLED" {
		http.Error(w, "Order already cancelled", http.StatusBadRequest)
		return
	}

	type itemToReturn struct {
		productID int
		qty       int
	}

	itemQuery := `
		SELECT p.id, oi.qty
		FROM sales_order_items oi
		JOIN products p ON oi.sku = p.sku
		WHERE oi.order_id = $1
		ORDER BY p.id ASC
	`
	rows, err := tx.QueryContext(r.Context(), itemQuery, orderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := []itemToReturn{}
	for rows.Next() {
		var itr itemToReturn
		if err := rows.Scan(&itr.productID, &itr.qty); err != nil {
			rows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, itr)
	}
	rows.Close()

	for _, item := range items {
		_, err = tx.ExecContext(r.Context(), "SELECT id FROM products WHERE id = $1 FOR UPDATE", item.productID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = tx.ExecContext(r.Context(), "UPDATE products SET stock = stock + $1 WHERE id = $2", item.qty, item.productID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cancelNote := fmt.Sprintf("Cancelled Sales Order #%d", orderID)
		_, err = tx.ExecContext(r.Context(),
			"INSERT INTO stock_movements (product_id, type, qty, note) VALUES ($1, 'in', $2, $3)",
			item.productID, item.qty, cancelNote)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	now := time.Now()
	_, err = tx.ExecContext(r.Context(), "UPDATE sales_orders SET status = 'CANCELLED', cancelled_at = $1 WHERE id = $2", now, orderID)
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
		"message":      "Order cancelled successfully and stock refunded",
		"order_id":     orderID,
		"status":       "CANCELLED",
		"cancelled_at": now,
	})
}
