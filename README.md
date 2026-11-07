# StockFlow — Real-Time Inventory & Sales Order Management System

StockFlow is a production-grade, full-stack inventory and sales order management system engineered to guarantee strict data consistency under high-concurrency workloads.

The system prevents race conditions and overselling using PostgreSQL row-level locking (`SELECT ... FOR UPDATE`), eliminates database deadlocks via deterministic lexicographical SKU sorting, handles automated multi-item order rollbacks, and maintains an immutable audit ledger for all stock movements.

---

## Key Features

- **Concurrency-Safe Stock Deduction**: Prevents race conditions and overselling during simultaneous checkout attempts using atomic PostgreSQL transactions with `SELECT ... FOR UPDATE`.
- **Deadlock Prevention**: Automatically sorts product SKUs lexicographically before acquiring row locks during multi-item order processing.
- **Stock Movement Audit Ledger**: Maintains a complete audit trail tracking all inbound (`in`) and outbound (`out`) movements (initial seeds, manual adjustments, sales fulfillments, and cancellation refunds).
- **Atomic Order Cancellation**: Cancels orders, restores reserved stock quantities, and logs corresponding ledger records within a single atomic database transaction.
- **Server-Side Pagination & Search**: Query filtering by product name or SKU alongside dynamic pagination for high-volume catalogs.
- **Responsive Dashboard UI**: Built with React 18, TypeScript, and Vite, featuring stock adjustment controls, real-time status tracking, and mutation history inspection.

---

## Tech Stack

| Component | Technology | Description |
|---|---|---|
| **Backend** | Go 1.23 | Routing via `go-chi/v5`, database connectivity via `lib/pq` |
| **Database** | PostgreSQL 16 | Relational persistence with ACID transactions and row locking |
| **Frontend** | React 18 | TypeScript, Vite, Lucide-React |
| **Infrastructure** | Docker & Compose | Multi-stage build containerization |

---

## Project Structure

```text
stockflow/
├── backend/
│   ├── Dockerfile             # Multi-stage build Go binary
│   ├── go.mod                 # Go module definition
│   ├── go.sum                 # Go checksum validation
│   ├── init.sql               # PostgreSQL schema & initial seed data
│   └── main.go                # REST API handlers, SQL transactions, & CORS middleware
├── frontend/
│   ├── public/                # Static assets
│   ├── src/
│   │   ├── App.tsx            # Main application UI (Catalog, Modals, & Orders)
│   │   ├── index.css          # Design system & component styling
│   │   ├── main.tsx           # React entry point
│   │   └── types.ts           # Shared TypeScript interfaces
│   ├── package.json           # Node.js dependencies & scripts
│   ├── tsconfig.json          # TypeScript compiler configuration
│   └── vite.config.ts         # Vite bundler configuration
├── docker-compose.yml         # Container orchestration (PostgreSQL & Backend API)
└── README.md                  # Project documentation
```

---

## Database Schema

The database consists of 4 relational tables:

1. **`products`**: Master catalog storing SKU, name, price, active status, and real-time available stock.
2. **`stock_movements`**: Immutable audit ledger recording inventory changes (`type: in/out`, `qty`, `note`, `created_at`).
3. **`sales_orders`**: Order headers tracking customer details, total value, creation time, and status (`CREATED` or `CANCELLED`).
4. **`sales_order_items`**: Line items linked to orders (`sku`, `qty`, `unit_price`).

---

## Environment Variables

The backend is configurable via the following environment variables:

| Variable | Default | Description |
|---|---|---|
| `DB_HOST` | `localhost` | PostgreSQL host address (`db` when running inside Docker) |
| `DB_PORT` | `5432` | PostgreSQL service port |
| `DB_USER` | `postgres` | Database username |
| `DB_PASSWORD` | `password` | Database password |
| `DB_NAME` | `stockflow` | Target database name |
| `PORT` | `8080` | HTTP backend server port |

---

## API Specification

### 1. Products & Stock Adjustments

| Method | Endpoint | Description | Success Status |
|---|---|---|---|
| `GET` | `/api/products?search=&page=1&limit=10` | Fetch products with pagination & search | `200 OK` |
| `POST` | `/api/products/{id}/stock` | Manual stock adjustment (`in` / `out`) | `200 OK` |
| `GET` | `/api/products/{id}/movements` | Retrieve ledger history for a product | `200 OK` |

### 2. Sales Orders

| Method | Endpoint | Description | Success Status |
|---|---|---|---|
| `POST` | `/api/orders` | Create an order with atomic multi-item locking | `201 Created` |
| `GET` | `/api/orders` | Retrieve list of all sales orders | `200 OK` |
| `POST` | `/api/orders/{id}/cancel` | Cancel order and refund reserved inventory | `200 OK` |

---

## Error Handling

When an order request exceeds available stock, the backend aborts the entire transaction and returns an HTTP `409 Conflict`:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "sku": "BEV-004",
    "requested": 999,
    "available": 15
  }
}
```

---

## Getting Started

### Option 1: Docker Compose (Recommended)

1. Start the PostgreSQL database and Go backend containers:
   ```bash
   docker compose up --build -d
   ```

2. Start the frontend development server:
   ```bash
   cd frontend
   npm install
   npm run dev
   ```

3. Open your browser and navigate to `http://localhost:5173`.

---

### Option 2: Local Manual Setup

1. **Database Setup**:
   Ensure PostgreSQL is running locally on port `5432`, create a database named `stockflow`, and execute the schema:
   ```bash
   psql -U postgres -d stockflow -f backend/init.sql
   ```

2. **Run Backend**:
   ```bash
   cd backend
   go run main.go
   ```
   The backend API will listen on `http://localhost:8080`.

3. **Run Frontend**:
   ```bash
   cd frontend
   npm install
   npm run dev
   ```
   The frontend will run on `http://localhost:5173`.

---

## Verification & Concurrency Testing

### 1. Standard Order Creation
```powershell
Invoke-RestMethod -Uri "http://localhost:8080/api/orders" -Method Post -ContentType "application/json" -Body '{"customer":"Kafe Nusantara","items":[{"sku":"BEV-001","qty":2},{"sku":"BEV-003","qty":3}]}'
```

### 2. Atomic Rollback on Insufficient Stock
Attempts to purchase an excess quantity. The transaction automatically rolls back, leaving all item balances unchanged:
```powershell
Invoke-RestMethod -Uri "http://localhost:8080/api/orders" -Method Post -ContentType "application/json" -Body '{"customer":"Warung Berkah","items":[{"sku":"BEV-001","qty":1},{"sku":"BEV-004","qty":999}]}'
```

### 3. Order Cancellation & Stock Refund
Cancels Sales Order #1, restores stock quantities, and appends refund entries into the movement ledger:
```powershell
Invoke-RestMethod -Uri "http://localhost:8080/api/orders/1/cancel" -Method Post -ContentType "application/json"
```

### 4. High-Concurrency Race Condition Simulation
Executes two simultaneous checkout requests targeting identical remaining inventory units in parallel background jobs:
```powershell
$body = '{"customer":"Race Condition Test","items":[{"sku":"BEV-005","qty":10}]}'
$job1 = Start-Job -ScriptBlock { param($b) Invoke-RestMethod -Uri "http://localhost:8080/api/orders" -Method Post -ContentType "application/json" -Body $b } -ArgumentList$body
$job2 = Start-Job -ScriptBlock { param($b) Invoke-RestMethod -Uri "http://localhost:8080/api/orders" -Method Post -ContentType "application/json" -Body $b } -ArgumentList$body

Receive-Job $job1 -Wait
Receive-Job $job2 -Wait
```
*Expected Result: One transaction acquires the lock and succeeds (`201 Created`), while the competing request is cleanly rejected (`409 Conflict - INSUFFICIENT_STOCK`) without inventory inconsistency or negative balances.*