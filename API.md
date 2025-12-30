Aşağıda **step-by-step** akış + **test örnekleri** eklenmiş, düzenli bir doküman var.
(API değişkenleri ve tırnak içleri aynen korunmuştur.)

---

## ✅ Step-by-step 工作流程（从下单到完成）

### Step 0 — 准备

* 你需要一个可用的 **Your API key**
* 你需要一个可访问 TikTok 视频数据的方式（用于抓取播放量）
* 订单数据至少包含：

  * link（视频链接）
  * quantity（需要发送的播放量）

---

### Step 1 — 客户端下单（action: "add"）

客户端调用你的 API，把订单发给你。

**请求参数：**

* key: Your API key
* action: "add"
* service: Service ID（不重要）
* link: Link to page（TikTok 视频链接）
* quantity: Quantity to be delivered

**你的服务要做的事：**

1. 从 link 解析 aweme id
2. 立即抓取视频当前播放量，保存为 "start_count"
3. 创建订单并返回 Order ID（order）

✅ **返回：**

* order: Order ID

---

### Step 2 — 后台开始执行订单

你的后台 worker/队列开始处理订单：

1. 读取订单（order, aweme id, quantity, start_count）
2. 执行播放量发送逻辑（内部实现）
3. 持续更新订单进度：

   * 已发送数量
   * remains（剩余数量）
   * status（状态机）

建议状态机：

* "Pending" → "In progress" → "Completed"
* 如部分失败：→ "Partial"
* 不可执行：→ "Canceled"

---

### Step 3 — 客户端查状态（action: "status", order）

客户端轮询查询单个订单状态。

**请求参数：**

* key: Your API key
* action: "status"
* order: Order ID

**你返回：**

* "charge"（可固定返回或忽略）
* "start_count"
* "status"
* "remains"

---

### Step 4 — 完成校验（必须）

当后台认为订单已完成时：

1. 再次抓取视频当前播放量（final_count）
2. 校验是否达到目标（建议规则举例）：

   * 目标播放量 = start_count + quantity
   * 如果 final_count >= 目标播放量：标记 "Completed"
   * 否则：标记 "Partial" 并更新 "remains"

---

### Step 5 — 批量查状态（action: "status", orders）

客户端一次性查多个订单状态，减少压力。

**请求参数：**

* key: Your API key
* action: "status"
* orders: Order IDs separated by comma

  * 示例：`47471,50750,51006,45135`

---

## 🧪 Test Examples（测试示例）

下面用通用形式写（GET/POST 都行，你按你实际实现决定）。
我这里用 **POST form-data / x-www-form-urlencoded** 举例。

---

### ✅ Example 1 — Add Order（下单）

**Request**

```
POST /api
Content-Type: application/x-www-form-urlencoded

key=YOUR_KEY&action=add&service=1&link=https://www.tiktok.com/@xx/video/123456&quantity=1000
```

**Expected Response**

```
{
  "order": "12421"
}
```

**Server-side checklist**

* 能解析 link
* 保存 start_count
* 创建订单记录
* 返回 order

---

### ✅ Example 2 — Single Status（单订单状态）

**Request**

```
POST /api
Content-Type: application/x-www-form-urlencoded

key=YOUR_KEY&action=status&order=12421
```

**Possible Response A — Pending**

```
{
  "charge": "0.00000",
  "start_count": "3572",
  "status": "Pending",
  "remains": "1000"
}
```

**Possible Response B — In progress**

```
{
  "charge": "0.00000",
  "start_count": "3572",
  "status": "In progress",
  "remains": "420"
}
```

**Possible Response C — Completed**

```
{
  "charge": "0.00000",
  "start_count": "3572",
  "status": "Completed",
  "remains": "0"
}
```

**Possible Response D — Partial**

```
{
  "charge": "0.00000",
  "start_count": "3572",
  "status": "Partial",
  "remains": "157"
}
```

**Possible Response E — Canceled**

```
{
  "charge": "0.00000",
  "start_count": "3572",
  "status": "Canceled",
  "remains": "1000"
}
```

---

### ✅ Example 3 — Multi Status（批量订单状态）

**Request**

```
POST /api
Content-Type: application/x-www-form-urlencoded

key=YOUR_KEY&action=status&orders=47471,50750,51006,45135
```

**Expected Response（建议格式，按订单ID返回对象）**

```
{
  "47471": {"charge":"0.00000","start_count":"100","status":"Completed","remains":"0"},
  "50750": {"charge":"0.00000","start_count":"230","status":"In progress","remains":"300"},
  "51006": {"charge":"0.00000","start_count":"50","status":"Pending","remains":"1000"},
  "45135": {"charge":"0.00000","start_count":"900","status":"Partial","remains":"120"}
}
```

---

## ⚙️ 并发与稳定性建议（重要）

为了支持“一次上百订单”甚至更高并发：

1. **Add Order 只做轻量工作**

   * 解析 link
   * 抓一次 start_count
   * 入队列/写数据库
   * 立即返回 order

2. **后台异步执行**

   * 用队列/worker（多进程/多机）
   * 每个订单定期更新 remains/status

3. **状态查询做缓存**

   * status 结果可缓存几秒，减少 DB 压力

4. **批量 status 优先**

   * 客户端尽量用 orders 批量查询，少打单个接口

