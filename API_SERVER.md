# api_server API 请求文档（详细版）

本文档基于仓库内 `api_server/` 的**当前实现**整理，按“逐接口/逐参数”描述，方便直接对接。

---

## 0. 基本信息
- **服务目录**：`api_server/`
- **默认监听**：`API_ADDR=:8080`
- **核心接口**：`POST /api`（`application/x-www-form-urlencoded`）
- **鉴权**：每次请求都必须携带 `key`（API Key）
  - API Key **来源 MySQL 表**：`api_keys`
  - API Key **缓存**：进程内 TTL cache（优先从 cache 校验；miss 回源 DB）
    - TTL：`API_KEY_CACHE_TTL_SEC`（默认 30 秒）

---

## 0.1 Content-Type 与请求方式

当前服务端会对每个请求执行 `ParseForm()`，所以以下两种形式都可以：

- **推荐**：`application/x-www-form-urlencoded`
- `multipart/form-data`（也可用）

---

## 1. 健康检查

### 1.1 `GET /healthz`

#### 请求参数
无

#### 响应
- **200 OK**
- body：`ok`

---

## 2. 核心 API（统一入口）

### 2.0 `POST /api`

#### 公共参数（所有 action 都必须传）

| 参数名 | 类型 | 必填 | 示例 | 说明 |
|---|---|---:|---|---|
| `key` | string | ✅ | `YOUR_KEY` | API Key（从 DB 校验，必须存在且启用；服务端会做进程内 TTL cache） |
| `action` | string | ✅ | `add` / `status` | 动作 |
| `service` | string/int | ❌ | `1` | 保留字段：当前实现不使用 |

#### 公共错误返回（所有 action 可能出现）

| HTTP | body | 触发条件 |
|---:|---|---|
| 401 | `{"error":"missing key"}` | 未传 `key` |
| 401 | `{"error":"invalid key"}` | `key` 在 DB 不存在 |
| 401 | `{"error":"key disabled"}` | `api_keys.is_active=0` |
| 500 | `{"error":"auth error"}` | 校验 key 时 DB 异常 |
| 400 | `{"error":"invalid action"}` | action 不是 `add/status` |

> 说明：本服务对每个 API 请求都会校验 `key`，并且 status 查询还会做 **订单归属校验**（只能查自己 key 创建的订单）。

---

## 2.1 下单接口

### 2.1.1 `POST /api`（`action=add`）

#### 请求参数

| 参数名 | 类型 | 必填 | 示例 | 说明 |
|---|---|---:|---|---|
| `key` | string | ✅ | `YOUR_KEY` | API Key |
| `action` | string | ✅ | `add` | 固定值 |
| `link` | string | ✅ | `https://www.tiktok.com/@xx/video/123456` | TikTok 视频链接（服务端解析 aweme_id） |
| `quantity` | int64 | ✅ | `1000` | 下单数量（必须 > 0） |
| `service` | string/int | ❌ | `1` | 当前实现不使用 |

#### 服务端行为（按顺序）
1. 从 `link` 解析 `aweme_id`
2. 抓取并保存 `start_count`（当前版本 `fetchStartCount()` 先返回 0）
3. **事务扣减额度**：
   - `SELECT api_keys.credit FOR UPDATE`
   - 校验 `credit >= quantity`
   - `UPDATE api_keys SET credit = credit - quantity`
4. `INSERT INTO orders(...)`
5. 成功后刷新服务端进程内 cache（避免缓存变脏）

#### 成功响应
- **200**

| 字段 | 类型 | 示例 | 说明 |
|---|---|---|---|
| `order` | string | `"12421"` | 订单号（自增 ID，字符串返回） |

#### 错误响应（add 专属）

| HTTP | body | 触发条件 |
|---:|---|---|
| 400 | `{"error":"invalid quantity"}` | quantity 非法（<=0 或不是整数） |
| 400 | `{"error":"invalid link"}` | link 解析 aweme_id 失败 |
| 400 | `{"error":"insufficient credit"}` | `api_keys.credit < quantity` |
| 500 | `{"error":"db error"}` | DB 写入/事务错误（非额度不足） |

#### curl 示例

```bash
curl -X POST "http://127.0.0.1:8080/api" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "key=YOUR_KEY&action=add&service=1&link=https://www.tiktok.com/@xx/video/123456&quantity=1000"
```

---

## 2.2 订单状态查询

### 2.2.1 `POST /api`（`action=status`，单订单）

#### 请求参数

| 参数名 | 类型 | 必填 | 示例 | 说明 |
|---|---|---:|---|---|
| `key` | string | ✅ | `YOUR_KEY` | API Key |
| `action` | string | ✅ | `status` | 固定值 |
| `order` | int64 | ✅ | `12421` | 订单号 |

#### 响应字段（成功）
> 注意：所有字段都是 **string**（兼容 `API.md` 示例）

| 字段 | 类型 | 示例 | 说明 |
|---|---|---|---|
| `charge` | string | `"0.00000"` | 固定值（当前实现） |
| `start_count` | string | `"3572"` | 订单创建时保存的 start_count |
| `status` | string | `"Pending"` / `"In progress"` / `"Completed"` / `"Partial"` / `"Canceled"` | 订单状态 |
| `remains` | string | `"420"` | 剩余数量（`quantity - delivered`，下限 0） |

#### 订单归属校验（非常重要）
服务端查询 SQL 为：`WHERE id = ? AND api_key = ?`  

因此：即使你有合法的 `key`，也**只能查询自己创建的订单**；否则会表现为 “not found”。

#### 错误响应（status 单订单）

| HTTP | body | 触发条件 |
|---:|---|---|
| 400 | `{"error":"invalid order"}` | order 非法 |
| 404 | `{"error":"order not found"}` | 订单不存在，或不属于当前 key |
| 500 | `{"error":"db error"}` | DB 查询异常 |

#### curl 示例

```bash
curl -X POST "http://127.0.0.1:8080/api" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "key=YOUR_KEY&action=status&order=12421"
```

---

### 2.2.2 `POST /api`（`action=status`，批量 orders）

#### 请求参数

| 参数名 | 类型 | 必填 | 示例 | 说明 |
|---|---|---:|---|---|
| `key` | string | ✅ | `YOUR_KEY` | API Key |
| `action` | string | ✅ | `status` | 固定值 |
| `orders` | string | ✅ | `47471,50750,51006` | 订单号列表（逗号分隔） |

#### 响应（成功）
- **200**
- 返回对象以订单号（字符串）作为 key，每个 value 为状态对象或错误对象。

状态对象字段（与单订单一致）：
- `charge`（string）
- `start_count`（string）
- `status`（string）
- `remains`（string）

当订单不存在/不属于该 key，会返回错误对象：

```json
{"error":"order not found"}
```

#### curl 示例

```bash
curl -X POST "http://127.0.0.1:8080/api" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "key=YOUR_KEY&action=status&orders=47471,50750,51006,45135"
```

---

## 3. 管理后台（新增/追加 API Key + 额度）

### 3.1 `GET /admin`

#### 作用
打开一个简单页面，用于新增/追加 `api_key` 与额度（credit）。

#### 请求参数
无

#### 响应
- **200**
- HTML 页面

---

### 3.2 `POST /admin/api_keys/add`

#### 说明
该接口用于**创建或更新** `api_keys` 表记录（并刷新服务端进程内 cache）。

#### 请求参数（表单）

| 参数名 | 类型 | 必填 | 示例 | 说明 |
|---|---|---:|---|---|
| `password` | string | ✅ | `123456` | 管理员密码（明文输入；后端做 md5 校验） |
| `api_key` | string | ✅ | `YOUR_KEY` | 要新增/更新的 API Key |
| `merchant_name` | string | ❌ | `merchant_a` | 商家名称（非空才会更新） |
| `credit_delta` | int64 | ✅ | `100000` | 增加额度（必须 > 0） |

#### 密码校验规则
- 服务端计算：`md5(password)`（hex 小写）
- 必须等于配置：`ADMIN_PASSWORD_MD5`
- 如果 `ADMIN_PASSWORD_MD5` 未配置，接口会返回错误

#### 写库规则（Upsert）
- key 不存在：创建并设置  
  - `is_active=1`
  - `credit=credit_delta`
  - `total_credit=credit_delta`
- key 已存在：更新  
  - `is_active=1`
  - `credit += credit_delta`
  - `total_credit += credit_delta`
  - `merchant_name`：仅当本次传入非空时更新

#### 缓存刷新
- 服务端会刷新进程内 cache（TTL 缓存），不需要额外中间件。

#### 成功响应
- **200**
- body：`ok`（text/html）

#### 错误响应

| HTTP | body（文本） | 触发条件 |
|---:|---|---|
| 401 | `invalid password` | 管理员密码 md5 不匹配 |
| 400 | `api_key is required` | api_key 为空 |
| 400 | `credit_delta must be > 0` | credit_delta 非法 |
| 500 | `ADMIN_PASSWORD_MD5 not set` | 未配置管理员密码 md5 |
| 500 | `db error: ...` | DB 写入失败 |

#### curl 示例

```bash
curl -X POST "http://127.0.0.1:8080/admin/api_keys/add" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "password=123456&api_key=YOUR_KEY&merchant_name=merchant_a&credit_delta=100000"
```

---

## 4. 数据结构（DB）

### 4.1 MySQL：`api_keys`
字段含义见 `api_server/schema.sql`，核心字段：
- `api_key`：API Key（主键）
- `merchant_name`：商家名称
- `is_active`：启用/禁用
- `credit`：当前可用额度（下单扣减）
- `total_credit`：累计额度（不扣减）

### 4.2 MySQL：`orders`
- `api_key`：订单归属（status 会按此做权限隔离）
- `quantity/delivered/start_count/status`

### 4.3 API Key 缓存（进程内 TTL）
- 用途：加速每次请求校验 key（跨进程不共享）
- TTL：`API_KEY_CACHE_TTL_SEC`


