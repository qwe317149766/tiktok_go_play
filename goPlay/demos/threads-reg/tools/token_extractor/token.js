const fs = require('fs');
const path = require('path');

// 1. 读取 data.txt
const filePath = path.join(__dirname, 'data.txt');
let rawData;

try {
    rawData = fs.readFileSync(filePath, 'utf8');
} catch (err) {
    console.error(`无法读取文件: ${filePath}`);
    process.exit(1);
}

/**
 * 核心逻辑：使用更健壮的正则来匹配高度转义的 JSON
 * 通过匹配 "键名" + "任意数量的反斜杠" + "冒号" + "任意数量的反斜杠" + "引号" 来获取值
 */
function extractValue(data, key, isNumber = false) {
    // 匹配模式说明:
    // 1. key (如 username)
    // 2. 1到32个反斜杠 (\\{1,32\})
    // 3. 引号 (")
    // 4. 冒号 (:)
    // 5. 又是 1到32个反斜杠 和 引号 (针对字符串值)
    // 6. 捕捉组 ([^"\\]+) 匹配非引号非反斜杠的内容

    let regex;
    if (isNumber) {
        // 数字通常没有引号包裹值
        regex = new RegExp(`${key}[\\\\\\"]+:[\\\\\\"]*(\\d+)`, 'i');
    } else {
        // 字符串被高度转义的引号包裹
        regex = new RegExp(`${key}[\\\\\\"]+:[\\\\\\s]*[\\\\\\"]+([^"\\\\]+)`, 'i');
    }

    const match = data.match(regex);
    return match ? match[1] : null;
}

console.log("=== 正在从 data.txt 提取信息 ===");

// 针对 data.txt 中的 Level 4 嵌套进行提取
const username = extractValue(rawData, "username");
const pkid = extractValue(rawData, "pk", true);
const nonce = extractValue(rawData, "session_flush_nonce") || extractValue(rawData, "partially_created_account_nonce");

// 特殊处理 Authorization, 包含 Bearer IGT:2: 前缀
const authRegex = /IG-Set-Authorization[\\"]+:[\\"\s]+Bearer\s+IGT:2:([^"\\\\]+)/i;
const authMatch = rawData.match(authRegex);
const token = authMatch ? authMatch[1] : null;

// 输出结果
console.log(`[+] 用户名 (Username): ${username || '未找到'}`);
console.log(`[+] 用户 ID (PKID):     ${pkid || '未找到'}`);
console.log(`[+] Session Nonce:      ${nonce || '未找到'}`);
console.log(`[+] 认证 Token (Part):  ${token ? token.substring(0, 30) + "..." : '未找到'}`);

if (token) {
    console.log("\n[完整 Authorization Header]");
    console.log(`Bearer IGT:2:${token}`);
}

// 如果全都失败了，尝试 JSON 解析寻找线索
if (!username && !pkid && !token) {
    console.log("\n[提示] 正则提取失败，尝试解析 JSON 结构...");
    try {
        const obj = JSON.parse(rawData);
        console.log("JSON 解析成功，但未能在顶层找到目标字段。");
    } catch (e) {
        console.log("JSON 格式不完整或解析失败。");
    }
}
