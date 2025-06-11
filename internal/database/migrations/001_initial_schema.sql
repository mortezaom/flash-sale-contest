-- Sales table
CREATE TABLE IF NOT EXISTS sales (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id VARCHAR(50) UNIQUE NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    total_items INTEGER NOT NULL DEFAULT 10000,
    items_sold INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Items table  
CREATE TABLE IF NOT EXISTS items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id VARCHAR(50) UNIQUE NOT NULL,
    sale_id VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    image_url VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Checkout attempts table
CREATE TABLE IF NOT EXISTS checkout_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(100) NOT NULL,
    item_id VARCHAR(50) NOT NULL,
    code VARCHAR(100) NOT NULL,
    status BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Purchases table
CREATE TABLE IF NOT EXISTS purchases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(100) NOT NULL,
    item_id VARCHAR(50) NOT NULL,
    purchase_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_sales_sale_id ON sales(sale_id);
CREATE INDEX IF NOT EXISTS idx_sales_status ON sales(status);
CREATE INDEX IF NOT EXISTS idx_items_sale_id ON items(sale_id);
CREATE INDEX IF NOT EXISTS idx_items_item_id ON items(item_id);
CREATE INDEX IF NOT EXISTS idx_checkout_attempts_sale_id ON checkout_attempts(sale_id);
CREATE INDEX IF NOT EXISTS idx_checkout_attempts_code ON checkout_attempts(code);
CREATE INDEX IF NOT EXISTS idx_purchases_sale_id ON purchases(sale_id);
CREATE INDEX IF NOT EXISTS idx_purchases_user_id ON purchases(user_id);