package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) EnsureSchema(ctx context.Context) error {
	// 内嵌建表（避免你忘了执行 schema.sql）
	const ddlKeys = `
CREATE TABLE IF NOT EXISTS api_keys (
  api_key VARCHAR(128) NOT NULL COMMENT 'Client API key (action requests use this)',
  merchant_name VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'Merchant name',
  is_active TINYINT(1) NOT NULL DEFAULT 1 COMMENT '1=active,0=disabled',
  credit BIGINT NOT NULL DEFAULT 0 COMMENT 'Play credits (quantity deducted on add)',
  total_credit BIGINT NOT NULL DEFAULT 0 COMMENT 'Total credited amount (lifetime, not deducted on add)',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Create time',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Update time',
  PRIMARY KEY (api_key),
  KEY idx_is_active (is_active) COMMENT 'Quick filter active keys'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='API keys + play credits';`

	const ddlOrders = `
CREATE TABLE IF NOT EXISTS orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'Order ID',
  order_id CHAR(36) NOT NULL COMMENT 'External order id (UUID), returned to client',
  api_key VARCHAR(128) NOT NULL COMMENT 'Which API key created this order',
  aweme_id VARCHAR(64) NOT NULL COMMENT 'Parsed aweme/video id',
  link TEXT NOT NULL COMMENT 'Original tiktok link',
  quantity BIGINT NOT NULL COMMENT 'Requested quantity',
  delivered BIGINT NOT NULL DEFAULT 0 COMMENT 'Delivered quantity (worker updates)',
  start_count BIGINT NOT NULL DEFAULT 0 COMMENT 'Start view count snapshot',
  status VARCHAR(32) NOT NULL DEFAULT 'Pending' COMMENT 'Pending/In progress/Completed/Partial/Canceled',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Create time',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Update time',
  PRIMARY KEY (id),
  UNIQUE KEY uk_order_id (order_id) COMMENT 'Unique external order id',
  KEY idx_api_key (api_key) COMMENT 'List orders by API key',
  KEY idx_aweme_id (aweme_id) COMMENT 'Search orders by aweme id',
  KEY idx_status (status) COMMENT 'Filter by status',
  KEY idx_created_at (created_at) COMMENT 'Time range queries'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Orders created via /api?action=add';`

	if _, err := r.db.ExecContext(ctx, ddlKeys); err != nil {
		return err
	}
	// 兼容老库：如果 api_keys 已存在但缺少 total_credit，则补列（忽略重复列错误）
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE api_keys ADD COLUMN total_credit BIGINT NOT NULL DEFAULT 0 COMMENT 'Total credited amount (lifetime, not deducted on add)'`); err != nil {
		// MySQL duplicate column name: Error 1060
		if !strings.Contains(err.Error(), "Duplicate column name") {
			return err
		}
	}
	// 兼容老库：补 merchant_name
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE api_keys ADD COLUMN merchant_name VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'Merchant name'`); err != nil {
		if !strings.Contains(err.Error(), "Duplicate column name") {
			return err
		}
	}

	// 兼容老库：orders 补 order_id + 唯一索引
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE orders ADD COLUMN order_id CHAR(36) NOT NULL COMMENT 'External order id (UUID), returned to client'`); err != nil {
		if !strings.Contains(err.Error(), "Duplicate column name") {
			return err
		}
	}
	// 可能已存在则忽略
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE orders ADD UNIQUE KEY uk_order_id (order_id)`); err != nil {
		// Duplicate key name: Error 1061
		if !strings.Contains(err.Error(), "Duplicate key name") {
			return err
		}
	}

	_, err := r.db.ExecContext(ctx, ddlOrders)
	return err
}

func (r *Repo) GetAPIKey(ctx context.Context, key string) (*APIKeyRow, error) {
	const q = `SELECT api_key, merchant_name, is_active, credit, total_credit, created_at, updated_at FROM api_keys WHERE api_key = ?`
	row := r.db.QueryRowContext(ctx, q, key)
	var k APIKeyRow
	var active int
	if err := row.Scan(&k.Key, &k.MerchantName, &active, &k.Credit, &k.TotalCredit, &k.CreatedAt, &k.UpdatedAt); err != nil {
		return nil, err
	}
	k.IsActive = active != 0
	return &k, nil
}

// UpsertAPIKeyAddCredit 新增或追加额度：
// - 若 api_key 不存在：创建并设置 credit=delta,total_credit=delta,is_active=1
// - 若存在：credit += delta, total_credit += delta；merchant_name 仅在传入非空时更新
func (r *Repo) UpsertAPIKeyAddCredit(ctx context.Context, apiKey string, merchantName string, creditDelta int64) error {
	const q = `
INSERT INTO api_keys (api_key, merchant_name, is_active, credit, total_credit)
VALUES (?, ?, 1, ?, ?)
ON DUPLICATE KEY UPDATE
  merchant_name = IF(VALUES(merchant_name) <> '', VALUES(merchant_name), merchant_name),
  is_active = 1,
  credit = credit + VALUES(credit),
  total_credit = total_credit + VALUES(total_credit)
`
	_, err := r.db.ExecContext(ctx, q, apiKey, merchantName, creditDelta, creditDelta)
	return err
}

func newUUIDv4() (string, error) {
	// RFC4122 v4
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	// 36 chars: 8-4-4-4-12
	hexs := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexs[0:8], hexs[8:12], hexs[12:16], hexs[16:20], hexs[20:32]), nil
}

// CreateOrderAndConsumeCredit 原子：检查额度并扣减，然后创建订单
func (r *Repo) CreateOrderAndConsumeCredit(ctx context.Context, apiKey, awemeID, link string, quantity, startCount int64) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	// 锁定这条 key
	var isActive int
	var credit int64
	if err := tx.QueryRowContext(ctx, `SELECT is_active, credit FROM api_keys WHERE api_key = ? FOR UPDATE`, apiKey).Scan(&isActive, &credit); err != nil {
		return "", err
	}
	if isActive == 0 {
		return "", fmt.Errorf("api key disabled")
	}
	if credit < quantity {
		return "", fmt.Errorf("insufficient credit")
	}

	if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET credit = credit - ? WHERE api_key = ?`, quantity, apiKey); err != nil {
		return "", err
	}

	orderID, err := newUUIDv4()
	if err != nil {
		return "", err
	}

	const q = `INSERT INTO orders (order_id, api_key, aweme_id, link, quantity, delivered, start_count, status) VALUES (?, ?, ?, ?, ?, 0, ?, 'Pending')`
	_, err = tx.ExecContext(ctx, q, orderID, apiKey, awemeID, link, quantity, startCount)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return orderID, nil
}

func (r *Repo) GetOrder(ctx context.Context, id int64) (*Order, error) {
	const q = `SELECT id, order_id, api_key, aweme_id, link, quantity, delivered, start_count, status, created_at, updated_at FROM orders WHERE id = ?`
	row := r.db.QueryRowContext(ctx, q, id)
	var o Order
	if err := row.Scan(&o.ID, &o.OrderID, &o.APIKey, &o.AwemeID, &o.Link, &o.Quantity, &o.Delivered, &o.StartCount, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repo) GetOrderForAPIKeyByOrderID(ctx context.Context, apiKey string, orderID string) (*Order, error) {
	const q = `SELECT id, order_id, api_key, aweme_id, link, quantity, delivered, start_count, status, created_at, updated_at FROM orders WHERE order_id = ? AND api_key = ?`
	row := r.db.QueryRowContext(ctx, q, orderID, apiKey)
	var o Order
	if err := row.Scan(&o.ID, &o.OrderID, &o.APIKey, &o.AwemeID, &o.Link, &o.Quantity, &o.Delivered, &o.StartCount, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repo) GetOrders(ctx context.Context, ids []int64) (map[int64]*Order, error) {
	out := make(map[int64]*Order, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	// 动态 IN
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	q := fmt.Sprintf(
		`SELECT id, api_key, aweme_id, link, quantity, delivered, start_count, status, created_at, updated_at FROM orders WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.APIKey, &o.AwemeID, &o.Link, &o.Quantity, &o.Delivered, &o.StartCount, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		oo := o
		out[o.ID] = &oo
	}
	return out, rows.Err()
}

func (r *Repo) GetOrdersForAPIKey(ctx context.Context, apiKey string, ids []int64) (map[int64]*Order, error) {
	out := make(map[int64]*Order, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, apiKey)
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	q := fmt.Sprintf(
		`SELECT id, order_id, api_key, aweme_id, link, quantity, delivered, start_count, status, created_at, updated_at
		 FROM orders
		 WHERE api_key = ? AND id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.OrderID, &o.APIKey, &o.AwemeID, &o.Link, &o.Quantity, &o.Delivered, &o.StartCount, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		oo := o
		out[o.ID] = &oo
	}
	return out, rows.Err()
}

func (r *Repo) GetOrdersForAPIKeyByOrderIDs(ctx context.Context, apiKey string, orderIDs []string) (map[string]*Order, error) {
	out := make(map[string]*Order, len(orderIDs))
	if len(orderIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, 0, len(orderIDs))
	args := make([]any, 0, len(orderIDs)+1)
	args = append(args, apiKey)
	for _, oid := range orderIDs {
		placeholders = append(placeholders, "?")
		args = append(args, oid)
	}

	q := fmt.Sprintf(
		`SELECT id, order_id, api_key, aweme_id, link, quantity, delivered, start_count, status, created_at, updated_at
		 FROM orders
		 WHERE api_key = ? AND order_id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.OrderID, &o.APIKey, &o.AwemeID, &o.Link, &o.Quantity, &o.Delivered, &o.StartCount, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		oo := o
		out[o.OrderID] = &oo
	}
	return out, rows.Err()
}

func withTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 3*time.Second)
}


