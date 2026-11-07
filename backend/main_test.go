package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
)

func setupTestRouter(t *testing.T) (*chi.Mux, *sql.DB) {
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dsn := fmt.Sprintf("host=%s port=5432 user=postgres password=password dbname=stockflow sslmode=disable", dbHost)

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open test DB connection: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("Database unreachable for testing: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Post("/products/{id}/stock", adjustStock)
		r.Post("/orders", createOrder)
	})

	return r, db
}

// Test 1: Happy path updates stock and appends immutable ledger movement
func TestAdjustStock_HappyPath(t *testing.T) {
	r, db := setupTestRouter(t)

	// Setup clean isolated product row
	var prodID int
	err := db.QueryRow(`
		INSERT INTO products (sku, name, price, stock, is_active)
		VALUES ('TEST-IN-01', 'Test Commodity In', 10000, 20, true)
		RETURNING id
	`).Scan(&prodID)
	if err != nil {
		t.Fatalf("Failed to insert fixture product: %v", err)
	}
	defer db.Exec("DELETE FROM products WHERE id = $1", prodID)

	payload := []byte(`{"type":"in","qty":5,"note":"Restock test"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%d/stock", prodID), bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got: %d", w.Code)
	}

	// Verify stock incremented to 25
	var updatedStock int
	if err := db.QueryRow("SELECT stock FROM products WHERE id = $1", prodID).Scan(&updatedStock); err != nil {
		t.Fatalf("Failed to read updated stock: %v", err)
	}
	if updatedStock != 25 {
		t.Errorf("Expected updated stock 25, got %d", updatedStock)
	}

	// Verify ledger entry exists
	var movementCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM stock_movements WHERE product_id = $1 AND type = 'in' AND qty = 5", prodID).Scan(&movementCount); err != nil {
		t.Fatalf("Failed to query stock_movements: %v", err)
	}
	if movementCount != 1 {
		t.Errorf("Expected 1 movement row written, found %d", movementCount)
	}
}

// Test 2: Insufficient stock rejected with NOTHING written (Rollback integrity)
func TestCreateOrder_InsufficientStock_NothingWritten(t *testing.T) {
	r, db := setupTestRouter(t)

	// Setup fixture: SKU with only 5 units in stock
	var prodID int
	err := db.QueryRow(`
		INSERT INTO products (sku, name, price, stock, is_active)
		VALUES ('TEST-OVERDRAW-01', 'Scarce Good', 50000, 5, true)
		RETURNING id
	`).Scan(&prodID)
	if err != nil {
		t.Fatalf("Failed to insert fixture product: %v", err)
	}
	defer db.Exec("DELETE FROM products WHERE id = $1", prodID)

	// Attempt to order 10 units (exceeds available 5)
	payload := []byte(`{"customer":"Overdraw Customer","items":[{"sku":"TEST-OVERDRAW-01","qty":10}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Assert 409 Conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("Expected status 409 Conflict, got: %d", w.Code)
	}

	var errBody struct {
		Error struct {
			Code      string `json:"code"`
			SKU       string `json:"sku"`
			Available int    `json:"available"`
		} `json:"error"`
	}
	json.NewDecoder(w.Body).Decode(&errBody)
	if errBody.Error.Code != "INSUFFICIENT_STOCK" || errBody.Error.Available != 5 {
		t.Errorf("Unexpected error envelope: %+v", errBody)
	}

	// Verify atomic rollback: Stock must remain untouched at 5
	var remainingStock int
	db.QueryRow("SELECT stock FROM products WHERE id = $1", prodID).Scan(&remainingStock)
	if remainingStock != 5 {
		t.Errorf("Data safety violation: stock changed to %d on rejected order", remainingStock)
	}

	// Verify no order rows were written
	var orderCount int
	db.QueryRow("SELECT COUNT(*) FROM sales_orders WHERE customer = 'Overdraw Customer'").Scan(&orderCount)
	if orderCount != 0 {
		t.Errorf("Rollback failed: sales_order row exists for failed transaction")
	}

	// Verify no movement rows were written
	var movementCount int
	db.QueryRow("SELECT COUNT(*) FROM stock_movements WHERE product_id = $1", prodID).Scan(&movementCount)
	if movementCount != 0 {
		t.Errorf("Rollback failed: stock_movement written for aborted order")
	}
}

// Test 3: Concurrency Race Guard - Stock never goes negative under simultaneous orders
func TestConcurrentOrders_NoNegativeStock(t *testing.T) {
	r, db := setupTestRouter(t)

	// Fixture with strictly 1 unit of stock
	var prodID int
	err := db.QueryRow(`
		INSERT INTO products (sku, name, price, stock, is_active)
		VALUES ('TEST-RACE-01', 'Single Unit Commodity', 75000, 1, true)
		RETURNING id
	`).Scan(&prodID)
	if err != nil {
		t.Fatalf("Failed to insert fixture: %v", err)
	}
	defer db.Exec("DELETE FROM products WHERE id = $1", prodID)

	const concurrentCallers = 10
	var wg sync.WaitGroup
	successCount := 0
	conflictCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrentCallers; i++ {
		wg.Add(1)
		go func(clientNum int) {
			defer wg.Done()

			payload := []byte(fmt.Sprintf(`{"customer":"Competitor %d","items":[{"sku":"TEST-RACE-01","qty":1}]}`, clientNum))
			req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			mu.Lock()
			defer mu.Unlock()
			if w.Code == http.StatusCreated {
				successCount++
			} else if w.Code == http.StatusConflict {
				conflictCount++
			}
		}(i)
	}

	wg.Wait()

	// Exactly 1 order must succeed; remaining 9 must receive 409 Conflict
	if successCount != 1 {
		t.Errorf("Expected exactly 1 order to succeed, got %d", successCount)
	}
	if conflictCount != (concurrentCallers - 1) {
		t.Errorf("Expected %d conflicts, got %d", concurrentCallers-1, conflictCount)
	}

	// Final stock must be exactly 0, never negative
	var finalStock int
	db.QueryRow("SELECT stock FROM products WHERE id = $1", prodID).Scan(&finalStock)
	if finalStock != 0 {
		t.Errorf("Stock balance corrupted: expected 0, got %d", finalStock)
	}
}
