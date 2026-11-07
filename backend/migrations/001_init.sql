CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    sku TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    price INTEGER NOT NULL CHECK (price >= 0),
    stock INTEGER NOT NULL DEFAULT 0 CHECK (stock >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stock_movements (
    id SERIAL PRIMARY KEY,
    product_id INTEGER NOT NULL REFERENCES products(id),
    type TEXT NOT NULL CHECK (type IN ('in', 'out')),
    qty INTEGER NOT NULL CHECK (qty > 0),
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sales_orders (
    id SERIAL PRIMARY KEY,
    customer TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'CREATED' CHECK (status IN ('CREATED', 'CANCELLED')),
    total INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at TIMESTAMPTZ
);

CREATE TABLE sales_order_items (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES sales_orders(id),
    sku TEXT NOT NULL REFERENCES products(sku),
    qty INTEGER NOT NULL CHECK (qty > 0),
    unit_price INTEGER NOT NULL
);

CREATE INDEX idx_products_sku ON products(sku);
CREATE INDEX idx_movements_product ON stock_movements(product_id, created_at DESC);

-- Seed Data: 12 Produk
INSERT INTO products (sku, name, price, stock, is_active) VALUES
('BEV-001', 'Biji Kopi Arabika Gayo 1kg', 185000, 25, TRUE),
('BEV-002', 'Biji Kopi Robusta Lampung 1kg', 95000, 30, TRUE),
('BEV-003', 'Susu Segar UHT Full Cream 1L', 19000, 48, TRUE),
('BEV-004', 'Oat Milk Barista Edition 1L', 42000, 15, TRUE),
('BEV-005', 'Sirup Karamel 750ml', 115000, 10, TRUE),
('BEV-006', 'Sirup Hazelnut 750ml', 115000, 8, TRUE),
('BEV-007', 'Matcha Pure Powder 500g', 140000, 12, TRUE),
('BEV-008', 'Cokelat Bubuk Premium 1kg', 85000, 20, TRUE),
('PAC-001', 'Paper Cup 8oz Panas (50 pcs)', 32000, 40, TRUE),
('PAC-002', 'Gelas Plastik Dingin 12oz (50 pcs)', 28000, 50, TRUE),
('PAC-003', 'Sedotan Kertas (100 pcs)', 15000, 100, TRUE),
('ACC-001', 'Kertas Filter V60 (100 pcs)', 45000, 5, TRUE);

-- Mutasi Awal Stok
INSERT INTO stock_movements (product_id, type, qty, note)
SELECT id, 'in', stock, 'Initial inventory seed' FROM products;