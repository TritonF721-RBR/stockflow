import React, { useState, useEffect } from 'react';
import { Package, ShoppingCart, RefreshCw, Eye, Plus, ArrowUpRight, ArrowDownLeft } from 'lucide-react';
import type { Product, StockMovement, SalesOrder } from './types';

const API_BASE = 'http://localhost:8080/api';

export default function App() {
  const [tab, setTab] = useState<'products' | 'orders'>('products');
  
  // Data State
  const [products, setProducts] = useState<Product[]>([]);
  const [orders, setOrders] = useState<SalesOrder[]>([]);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const limit = 5;

  // Modal State
  const [stockModal, setStockModal] = useState<{ open: boolean; product: Product | null }>({ open: false, product: null });
  const [movementModal, setMovementModal] = useState<{ open: boolean; product: Product | null; items: StockMovement[] }>({ open: false, product: null, items: [] });
  const [orderModal, setOrderModal] = useState(false);

  // Form Fields
  const [adjType, setAdjType] = useState<'in' | 'out'>('in');
  const [adjQty, setAdjQty] = useState(1);
  const [adjNote, setAdjNote] = useState('');
  
  const [customerName, setCustomerName] = useState('');
  const [orderItems, setOrderItems] = useState<{ sku: string; qty: number }[]>([{ sku: '', qty: 1 }]);

  useEffect(() => {
    fetchProducts();
  }, [search, page]);

  useEffect(() => {
    if (tab === 'orders') fetchOrders();
  }, [tab]);

  const fetchProducts = async () => {
    try {
      const res = await fetch(`${API_BASE}/products?search=${encodeURIComponent(search)}&page=${page}&limit=${limit}`);
      const data = await res.json();
      setProducts(data.data || []);
      setTotal(data.total || 0);
    } catch (err) {
      console.error('Failed to load products', err);
    }
  };

  const fetchOrders = async () => {
    try {
      const res = await fetch(`${API_BASE}/orders`);
      const data = await res.json();
      setOrders(data.data || []);
    } catch (err) {
      console.error('Failed to load orders', err);
    }
  };

  const handleAdjustStock = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!stockModal.product) return;

    try {
      const res = await fetch(`${API_BASE}/products/${stockModal.product.id}/stock`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: adjType, qty: Number(adjQty), note: adjNote }),
      });

      if (!res.ok) {
        const errorData = await res.json();
        alert(`Failed: ${errorData.error?.code || 'System error occurred'}`);
        return;
      }

      setStockModal({ open: false, product: null });
      setAdjQty(1);
      setAdjNote('');
      fetchProducts();
    } catch (err) {
      alert('Failed to reach the server.');
    }
  };

  const handleViewMovements = async (product: Product) => {
    try {
      const res = await fetch(`${API_BASE}/products/${product.id}/movements`);
      const data = await res.json();
      setMovementModal({ open: true, product, items: data.data || [] });
    } catch (err) {
      alert('Failed to retrieve movement history.');
    }
  };

  const handleCreateOrder = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const validItems = orderItems.filter((i) => i.sku && i.qty > 0);
      if (!customerName || validItems.length === 0) {
        alert('Please provide a customer name and at least one item.');
        return;
      }

      const res = await fetch(`${API_BASE}/orders`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ customer: customerName, items: validItems }),
      });

      if (!res.ok) {
        const err = await res.json();
        alert(`Order Rejected: ${err.error?.code} (SKU: ${err.error?.sku})`);
        return;
      }

      setOrderModal(false);
      setCustomerName('');
      setOrderItems([{ sku: '', qty: 1 }]);
      fetchOrders();
      fetchProducts();
    } catch (err) {
      alert('Failed to reach the server.');
    }
  };

  const handleCancelOrder = async (orderId: number) => {
    if (!confirm(`Cancel Sales Order #${orderId}? Reserved stock will be refunded automatically.`)) return;

    try {
      const res = await fetch(`${API_BASE}/orders/${orderId}/cancel`, { method: 'POST' });
      if (!res.ok) throw new Error();
      fetchOrders();
      fetchProducts();
    } catch (err) {
      alert('Failed to cancel order.');
    }
  };

  return (
    <div className="container">
      <header>
        <div className="brand">
          <Package size={28} />
          <span>StockFlow</span>
        </div>
        <div className="nav-tabs">
          <button className={`tab-btn ${tab === 'products' ? 'active' : ''}`} onClick={() => setTab('products')}>
            Product Catalog & Stock
          </button>
          <button className={`tab-btn ${tab === 'orders' ? 'active' : ''}`} onClick={() => setTab('orders')}>
            Sales Orders
          </button>
        </div>
      </header>

      {tab === 'products' ? (
        <div className="card">
          <div className="toolbar">
            <input
              type="text"
              placeholder="Search SKU or product name..."
              className="search-input"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(1);
              }}
            />
            <button className="btn btn-secondary" onClick={fetchProducts}>
              <RefreshCw size={14} /> Refresh
            </button>
          </div>

          <table>
            <thead>
              <tr>
                <th>SKU</th>
                <th>Product Name</th>
                <th>Price</th>
                <th>Stock</th>
                <th>Status</th>
                <th>Last Movement</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {products.map((p) => (
                <tr key={p.id}>
                  <td><strong>{p.sku}</strong></td>
                  <td>{p.name}</td>
                  <td>Rp {p.price.toLocaleString()}</td>
                  <td><strong>{p.stock}</strong></td>
                  <td>
                    <span className={`badge ${p.is_active ? 'badge-active' : 'badge-inactive'}`}>
                      {p.is_active ? 'Active' : 'Inactive'}
                    </span>
                  </td>
                  <td>{p.last_movement_at ? new Date(p.last_movement_at).toLocaleString() : '-'}</td>
                  <td>
                    <div style={{ display: 'flex', gap: '6px' }}>
                      <button
                        className="btn btn-sm btn-secondary"
                        onClick={() => setStockModal({ open: true, product: p })}
                      >
                        Adjust
                      </button>
                      <button className="btn btn-sm btn-secondary" onClick={() => handleViewMovements(p)}>
                        <Eye size={12} /> Movements
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <div className="pagination">
            <span>Page {page} of {Math.ceil(total / limit) || 1} ({total} total products)</span>
            <div style={{ display: 'flex', gap: '6px' }}>
              <button className="btn btn-sm btn-secondary" disabled={page === 1} onClick={() => setPage(page - 1)}>
                Previous
              </button>
              <button className="btn btn-sm btn-secondary" disabled={page * limit >= total} onClick={() => setPage(page + 1)}>
                Next
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div className="card">
          <div className="toolbar">
            <h3 style={{ fontSize: '16px' }}>Sales Orders Directory</h3>
            <button className="btn" onClick={() => setOrderModal(true)}>
              <Plus size={14} /> New Sales Order
            </button>
          </div>

          <table>
            <thead>
              <tr>
                <th>Order ID</th>
                <th>Customer</th>
                <th>Status</th>
                <th>Total Amount</th>
                <th>Created At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {orders.map((o) => (
                <tr key={o.id}>
                  <td><strong>#{o.id}</strong></td>
                  <td>{o.customer}</td>
                  <td>
                    <span className={`badge ${o.status === 'CREATED' ? 'badge-created' : 'badge-cancelled'}`}>
                      {o.status}
                    </span>
                  </td>
                  <td>Rp {o.total.toLocaleString()}</td>
                  <td>{new Date(o.created_at).toLocaleString()}</td>
                  <td>
                    {o.status === 'CREATED' && (
                      <button className="btn btn-sm btn-danger" onClick={() => handleCancelOrder(o.id)}>
                        Cancel
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* MODAL ADJUST STOCK */}
      {stockModal.open && stockModal.product && (
        <div className="modal-backdrop">
          <div className="modal-content">
            <h3 style={{ marginBottom: '16px' }}>Adjust Stock: {stockModal.product.name}</h3>
            <form onSubmit={handleAdjustStock}>
              <div className="form-group">
                <label>Adjustment Type</label>
                <select value={adjType} onChange={(e) => setAdjType(e.target.value as 'in' | 'out')}>
                  <option value="in">Inbound (Restock / In)</option>
                  <option value="out">Outbound (Write-off / Out)</option>
                </select>
              </div>
              <div className="form-group">
                <label>Quantity</label>
                <input
                  type="number"
                  min="1"
                  value={adjQty}
                  onChange={(e) => setAdjQty(Number(e.target.value))}
                  required
                />
              </div>
              <div className="form-group">
                <label>Note / Reason</label>
                <input
                  type="text"
                  placeholder="e.g., Damaged item write-off"
                  value={adjNote}
                  onChange={(e) => setAdjNote(e.target.value)}
                  required
                />
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setStockModal({ open: false, product: null })}>
                  Close
                </button>
                <button type="submit" className="btn">
                  Apply Adjustment
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL MOVEMENT HISTORY */}
      {movementModal.open && movementModal.product && (
        <div className="modal-backdrop">
          <div className="modal-content" style={{ maxWidth: '650px' }}>
            <h3 style={{ marginBottom: '16px' }}>Movement History: {movementModal.product.name}</h3>
            <div style={{ maxHeight: '350px', overflowY: 'auto' }}>
              <table>
                <thead>
                  <tr>
                    <th>Type</th>
                    <th>Qty</th>
                    <th>Note / Reference</th>
                    <th>Timestamp</th>
                  </tr>
                </thead>
                <tbody>
                  {movementModal.items.map((m) => (
                    <tr key={m.id}>
                      <td>
                        <span className={`badge ${m.type === 'in' ? 'badge-in' : 'badge-out'}`}>
                          {m.type === 'in' ? <ArrowDownLeft size={10} /> : <ArrowUpRight size={10} />} {m.type.toUpperCase()}
                        </span>
                      </td>
                      <td><strong>{m.qty}</strong></td>
                      <td>{m.note}</td>
                      <td>{new Date(m.created_at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setMovementModal({ open: false, product: null, items: [] })}>
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL CREATE SALES ORDER */}
      {orderModal && (
        <div className="modal-backdrop">
          <div className="modal-content">
            <h3 style={{ marginBottom: '16px' }}>Create Sales Order</h3>
            <form onSubmit={handleCreateOrder}>
              <div className="form-group">
                <label>Customer Name</label>
                <input
                  type="text"
                  placeholder="e.g., Nusantara Coffee Roasters"
                  value={customerName}
                  onChange={(e) => setCustomerName(e.target.value)}
                  required
                />
              </div>

              <div style={{ marginBottom: '12px' }}>
                <label style={{ fontSize: '13px', fontWeight: 600 }}>Order Items</label>
                {orderItems.map((item, idx) => (
                  <div key={idx} style={{ display: 'flex', gap: '8px', marginTop: '8px' }}>
                    <select
                      style={{ flex: 2, padding: '8px', borderRadius: '6px', border: '1px solid var(--border)' }}
                      value={item.sku}
                      onChange={(e) => {
                        const next = [...orderItems];
                        next[idx].sku = e.target.value;
                        setOrderItems(next);
                      }}
                      required
                    >
                      <option value="">Select Product (SKU)</option>
                      {products.map((p) => (
                        <option key={p.id} value={p.sku}>
                          {p.sku} - {p.name} (Stock: {p.stock})
                        </option>
                      ))}
                    </select>
                    <input
                      type="number"
                      min="1"
                      style={{ flex: 1, padding: '8px', borderRadius: '6px', border: '1px solid var(--border)' }}
                      value={item.qty}
                      onChange={(e) => {
                        const next = [...orderItems];
                        next[idx].qty = Number(e.target.value);
                        setOrderItems(next);
                      }}
                      required
                    />
                  </div>
                ))}
              </div>

              <button
                type="button"
                className="btn btn-sm btn-secondary"
                style={{ marginBottom: '16px' }}
                onClick={() => setOrderItems([...orderItems, { sku: '', qty: 1 }])}
              >
                + Add Item
              </button>

              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setOrderModal(false)}>
                  Cancel
                </button>
                <button type="submit" className="btn">
                  Submit Order
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}