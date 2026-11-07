# StockFlow — Inventory & Sales Order Management System

A production-grade, concurrency-safe inventory and sales order processing vertical slice built with **Go**, **PostgreSQL 16**, and **React + TypeScript (Vite)**.

---

## Architecture & Tech Stack

- **Backend**: Go (Chi Router, standard `database/sql` driver `github.com/lib/pq`)
- **Database**: PostgreSQL 16 Alpine (with strict relational constraints and seed data)
- **Frontend**: React 18, TypeScript (Strict Mode), Vite
- **Orchestration**: Docker Compose (multi-stage Go container + initialized Postgres database)

---

## Quick Start (Runnable in ≤ 5 Minutes)

### Prerequisites
- Docker & Docker Compose
- Node.js ≥ 18 & npm

### Step 1: Start Backend & Database
From the root project directory, build and launch the containerized database and Go API:
```bash
docker compose up --build -d
```
The database will automatically initialize schemas, constraints, indexes, 12 realistic seed products, and sample sales orders via `001_init.sql`.

- **API Base URL**: `http://localhost:8080/api`
- **Health Check / Products**: `http://localhost:8080/api/products`

### Step 2: Start Frontend Application
Open a new terminal window, navigate to the frontend directory, install dependencies, and start the local development server:
```bash
cd frontend
npm install
npm run dev
```
Open your browser at `http://localhost:5173`.

---

## Automated Tests

Automated tests cover atomic transactions, rollback guarantees on insufficient stock, and high-concurrency race condition safety.

To execute the test suite with Go's race detector enabled:
```bash
cd backend
go test -v -race ./...
```

### Test Coverage Highlights
1. `TestAdjustStock_HappyPath`: Validates stock updates and audit ledger entries.
2. `TestCreateOrder_InsufficientStock_NothingWritten`: Proves that when stock is insufficient, the entire transaction rolls back atomically—leaving product stock untouched and writing zero rows to `sales_orders` or `stock_movements`.
3. `TestConcurrentOrders_NoNegativeStock`: Simulates 10 concurrent requests competing for a single unit of stock. Exactly 1 request succeeds (201 Created), 9 requests are rejected with 409 Conflict, and stock is guaranteed to never drop below zero.

---

## API Contract & cURL Examples

### 1. Get Products (Search, Filter, Pagination)
```bash
curl -X GET "http://localhost:8080/api/products?search=kopi&filter=active&page=1&limit=10"
```

### 2. Create Product
```bash
curl -X POST "http://localhost:8080/api/products" \
  -H "Content-Type: application/json" \
  -d '{"sku":"ACC-002","name":"Coffee Dripper V60 02 Ceramic","price":175000,"is_active":true}'
```

### 3. Adjust Stock (Audit Ledger Mutation)
```bash
curl -X POST "http://localhost:8080/api/products/1/stock" \
  -H "Content-Type: application/json" \
  -d '{"type":"in","qty":10,"note":"Warehouse restock from supplier"}'
```

### 4. Create Sales Order (Atomic Multi-Item Checkout)
```bash
curl -X POST "http://localhost:8080/api/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "customer": "Kopi Toko Djawa",
    "items": [
      {"sku": "BEV-001", "qty": 2},
      {"sku": "BEV-003", "qty": 4}
    ]
  }'
```

### 5. Get Order Details
```bash
curl -X GET "http://localhost:8080/api/orders/1"
```

### 6. Cancel Sales Order (Stock Restoration)
```bash
curl -X POST "http://localhost:8080/api/orders/1/cancel"
```

---

## Key Design Decisions

### 1. Concurrency Control & Overselling Prevention
- **Row-Level Locking**: Implemented PostgreSQL pessimistic locking via `SELECT ... FOR UPDATE` within an explicit transaction block (`tx.Begin()`). We intentionally rejected a naive `SELECT-then-UPDATE` pattern to eliminate read skew under high concurrent traffic.
- **Deterministic SKU Sorting (Deadlock Prevention)**: If two concurrent transactions attempt to lock the same multiple SKUs in different sequences (e.g., Transaction A locks `[SKU-1, SKU-2]` while Transaction B locks `[SKU-2, SKU-1]`), PostgreSQL can deadlock. To prevent this, incoming order items are sorted lexicographically by SKU before acquiring locks.
- **Database CHECK Constraints**: Added `CHECK (stock >= 0)` and `CHECK (price >= 0)` as an unbreachable defense-in-depth barrier at the storage layer.

### 2. Single Source of Truth for `last_movement_at`
- Rather than maintaining a mutable timestamp column on the `products` table (which risks state drift if a ledger insert fails), `last_movement_at` is derived dynamically via an aggregated `LEFT JOIN` on the immutable `stock_movements` ledger:
```sql
LEFT JOIN (
    SELECT product_id, MAX(created_at) as last_movement
    FROM stock_movements
    GROUP BY product_id
) m ON p.id = m.product_id
```

### 3. Monetary Representation
- All monetary values are represented as integers in Indonesian Rupiah (IDR), eliminating floating-point rounding errors.

### 4. Frontend Architecture
- **Centralized API Client**: All HTTP operations, header configurations, and structured error envelopes are encapsulated inside `src/api/client.ts`. UI components interact strictly via typed domain models.
- **Strict TypeScript & No `any`**: Domain entities and API error envelopes are modeled strictly. Type-only imports (`import type`) are enforced to comply with modern ES module bundlers and `verbatimModuleSyntax`.
- **User-Centric Error Surfacing**: Backend `409 INSUFFICIENT_STOCK` errors explicitly render the problematic SKU and exact remaining stock directly inside the UI modal rather than merely logging to the browser console.

---

## What I Would Improve Next

1. **Idempotency-Key Support**: Implement an `Idempotency-Key` HTTP header on `POST /api/orders` cached via Redis or PostgreSQL to prevent accidental duplicate charges from client network retries.
2. **Containerized Frontend**: Incorporate a multi-stage Nginx container for the frontend into `docker-compose.yml` to allow zero-dependency single-command boot for non-developer reviewers.
3. **Frontend Testing Suite**: Add Vitest and React Testing Library integration tests for modal forms, debounced search triggers, and error boundary behaviors.
4. **Read Replica & Caching**: Cache product catalog responses using Redis with cache invalidation triggered on stock ledger inserts.

---

## Honest Hours Spent

- **System Architecture & Relational Schema (PostgreSQL)**: 0.5 hour
- **Go Backend Core, Concurrency Guards & Transactions**: 1.5 hours
- **React Frontend Implementation & API Client Integration**: 1.0 hour
- **Automated Testing (`go test -race`) & Verification**: 0.5 hour
- **Dockerization, Troubleshooting & Documentation**: 0.5 hour
- **Total Time**: ~4.0 hours (Completed in a single focused session from 13:00 to 17:00)