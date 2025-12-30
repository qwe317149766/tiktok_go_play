-- MySQL schema for tiktok_play_api
-- 默认库名：tiktok_play
--
-- 说明：
-- 1) api_keys：存放 API_KEY 与播放额度（credit）
-- 2) orders：下单记录（action=add），会在创建订单时扣减 credit

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='API keys + play credits';

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Orders created via /api?action=add';


