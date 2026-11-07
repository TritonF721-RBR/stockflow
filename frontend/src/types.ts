export interface Product {
  id: number;
  sku: string;
  name: string;
  price: number;
  stock: number;
  is_active: boolean;
  last_movement_at: string | null;
  created_at: string;
}

export interface ProductsResponse {
  data: Product[];
  page: number;
  limit: number;
  total: number;
}

export interface StockMovement {
  id: number;
  product_id: number;
  type: 'in' | 'out';
  qty: number;
  note: string;
  created_at: string;
}

export interface OrderItem {
  id: number;
  sku: string;
  qty: number;
  unit_price: number;
  subtotal: number;
}

export interface SalesOrder {
  id: number;
  customer: string;
  status: 'CREATED' | 'CANCELLED';
  total: number;
  created_at: string;
  cancelled_at: string | null;
}