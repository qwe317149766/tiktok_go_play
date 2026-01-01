# TikTok 邮箱批量注册工具

支持异步并发注册多个 TikTok 账号的工具。

## 功能特点

- ✅ 支持异步并发注册（可配置并发数）
- ✅ 自动轮询使用设备和代理
- ✅ 实时显示注册进度
- ✅ 自动保存注册结果
- ✅ 统计成功/失败数量

## 使用方法

### 1. 准备文件

在 `dgemail` 目录下创建以下文件：

#### `accounts.txt` - 账号列表
格式：每行一个账号，格式为 `email:password` 或 `email,password`

```
test1@gmail.com:password123
test2@gmail.com:password456
test3@gmail.com:password789
```

#### `devices.txt` - 设备列表
格式：每行一个设备的 JSON 字符串

```
{"device_id":"7584765107970262541","install_id":"7584766379061888782","ua":"com.zhiliaoapp.musically/2024204030 (Linux; U; Android 15; en_US; Pixel 6; Build/BP1A.250505.005; Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30)","openudid":"e3d21dd98be547de","cdid":"50263f00-94ce-425c-bf0f-a31520c77b93","device_guard_data0":"{\"device_token\":\"1|{\\\"aid\\\":1233,\\\"av\\\":\\\"42.4.3\\\",\\\"did\\\":\\\"7584765107970262541\\\",\\\"iid\\\":\\\"7584766379061888782\\\",\\\"fit\\\":\\\"1765966075\\\",\\\"s\\\":1,\\\"idc\\\":\\\"useast8\\\",\\\"ts\\\":\\\"1765966086\\\"}\",\"dtoken_sign\":\"ts.1.MEQCIH8xFZlELlawUJuS2VZy+XzCAQrJV4yfpCx/yJxmZJHNAiBmqAPFNSGlIgSwQF9KJs56MhHj9U3Dr+5UdayqPfjZXg==\"}"}
{"device_id":"7584765107970262542","install_id":"7584766379061888783","ua":"...","openudid":"...","cdid":"...","device_guard_data0":"..."}
```

#### （推荐）从 MySQL 读取设备（全 DB 模式）
本项目已改为全 DB：设备池与 cookies 池都使用 MySQL。

- 设备池表：`device_pool_devices`（默认；可用 `DB_DEVICE_POOL_TABLE` 覆盖）
- cookies 池表：`startup_cookie_accounts`（默认；可用 `DB_COOKIE_POOL_TABLE` 覆盖）
- 配置开启：`SIGNUP_DEVICES_SOURCE=db`
- 可选限制：`DEVICES_LIMIT`

#### `proxies.txt` - 代理列表
格式：每行一个代理地址

```
socks5h://proxy1:port@host:port
socks5h://proxy2:port@host:port
http://proxy3:port
```

> 说明：当前版本会优先读取“仓库根目录”的 `proxies.txt`（从当前目录向上查找，取最顶层那个），用于所有项目统一代理配置。

更新（代理读取优先级已调整）：
- 优先读取 **dgemail 自己目录下**的 `proxies.txt` 或 `data/proxies.txt`
- 如需显式指定路径，可在 `env.linux/env.windows` 里配置：
  - `PROXIES_FILE=/path/to/proxies.txt`（推荐）
  - 或 `SIGNUP_PROXIES_FILE=/path/to/proxies.txt`
- 只有当本地未找到时，才会兜底向上查找仓库根目录的 `proxies.txt`

### 2. 编译运行

```bash
cd go/demos/signup/dgemail
go build -o email_register.exe
./email_register.exe
```

或者直接运行：

```bash
go run main.go
```

### 3. 配置并发数

在 `main.go` 中修改 `maxConcurrency` 变量：

```go
maxConcurrency := 10  // 修改为你想要的并发数
```

## 输出文件

程序运行后会生成以下文件：

- `register_results.json` - 所有注册结果的详细信息（JSON格式）
- `success_accounts.txt` - 成功注册的账号列表（格式：email:username）
- `failed_accounts.txt` - 注册失败的账号列表（格式：email - 错误信息）

## 注意事项

1. **设备数量**：建议设备数量 >= 并发数，避免设备重复使用
2. **代理数量**：建议代理数量 >= 并发数，避免代理重复使用
3. **并发数**：建议设置为 5-20，过高可能导致请求失败
4. **设备格式**：确保 `device_guard_data0` 是有效的 JSON 字符串
5. **代理格式**：支持 `socks5h://`、`http://`、`https://` 格式

## 示例输出

```
=== TikTok 邮箱批量注册工具 ===

已加载 10 个账号
已加载 20 个设备
已加载 15 个代理
并发数: 10

[1/10] 开始注册: test1@gmail.com (设备: 7584765107970262541, 代理: socks5h://...)
[2/10] 开始注册: test2@gmail.com (设备: 7584765107970262542, 代理: socks5h://...)
...
[1/10] ✅ 成功: test1@gmail.com (用户名: testuser1)
[2/10] ❌ 失败: test2@gmail.com - 注册失败: ...

=== 注册完成 ===
总账号数: 10
成功: 8
失败: 2
耗时: 1m30s
平均速度: 0.11 账号/秒

结果已保存到: register_results.json
成功账号已保存到: success_accounts.txt
失败账号已保存到: failed_accounts.txt
```

