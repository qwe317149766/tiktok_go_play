# XBogus 和 XGnarly Go 语言实现

本目录包含 `xbogus.js` 和 `xgnarly.js` 的 Go 语言实现，确保输出与 JavaScript 版本完全一致。

## 文件说明

- `xbogus.go` - XBogus 算法的 Go 实现
- `xgnarly.go` - XGnarly 算法的 Go 实现
- `xbogus.js` - 原始 JavaScript 实现（参考）
- `xgnarly.js` - 原始 JavaScript 实现（参考）
- `xbogus_test.go` - XBogus 单元测试
- `xgnarly_test.go` - XGnarly 单元测试
- `compare_test.go` - 与 JavaScript 版本的对比测试

## 使用方法

### XBogus

```go
import "your-package/xbogus"

result := xbogus.Encrypt(
    "device_platform=android&os=android",  // params
    "{}",                                   // postData
    "Mozilla/5.0",                          // userAgent
    1234567890,                             // timestamp (uint32)
)
```

### XGnarly

```go
import "your-package/xgnarly"

result, err := xgnarly.EncryptXgnarly(
    "device_platform=android&os=android",  // queryString
    "{}",                                   // body
    "Mozilla/5.0",                          // userAgent
    0,                                      // envcode
    "5.1.1",                                // version ("5.1.0" or "5.1.1")
    1234567890000,                          // timestampMs (int64)
)
```

## 运行测试

### 基本测试

```bash
cd api_server
go test -v ./xbogus_test.go ./xbogus.go
go test -v ./xgnarly_test.go ./xgnarly.go
```

### 与 JavaScript 版本对比测试

需要安装 Node.js：

```bash
# 确保 Node.js 已安装
node --version

# 运行对比测试
go test -v ./compare_test.go ./xbogus.go ./xgnarly.go
```

## 注意事项

1. **XGnarly 的随机性**：XGnarly 使用伪随机数生成器（PRNG），每次运行可能产生不同的输出。要获得一致的结果，需要：
   - 使用相同的 `timestampMs` 参数
   - 确保 PRNG 状态初始化一致（通过 `ResetPrngState` 函数）

2. **线程安全**：XGnarly 的实现是线程安全的，使用了互斥锁保护全局状态。

3. **输出一致性**：由于 XGnarly 包含随机元素，完全一致的输出需要：
   - 相同的输入参数
   - 相同的 PRNG 初始状态
   - 相同的执行顺序

## 算法说明

### XBogus
- 使用自定义 Base64 编码
- MD5 哈希（双重哈希）
- RC4 加密
- XOR 校验和

### XGnarly
- 使用 ChaCha 加密算法
- 自定义 Base64 编码
- 伪随机数生成器（基于 ChaCha）
- 支持版本 5.1.0 和 5.1.1

## 调试

如果输出不一致，请检查：
1. 输入参数是否完全相同
2. 时间戳是否一致
3. PRNG 状态是否相同（对于 XGnarly）
4. 字符编码是否正确（UTF-8）

