#!/bin/bash

# 验证 Go 和 JavaScript 版本输出一致性的脚本

echo "=== 验证 XBogus 输出一致性 ==="

# 测试用例
PARAMS="device_platform=android&os=android"
POST_DATA="{}"
USER_AGENT="Mozilla/5.0"
TIMESTAMP=1234567890

echo "输入参数:"
echo "  params: $PARAMS"
echo "  postData: $POST_DATA"
echo "  userAgent: $USER_AGENT"
echo "  timestamp: $TIMESTAMP"
echo ""

# JavaScript 版本
echo "运行 JavaScript 版本..."
JS_OUTPUT=$(node -e "
const xbogus = require('./xbogus.js');
const result = xbogus('$PARAMS', '$POST_DATA', '$USER_AGENT', $TIMESTAMP);
console.log(result);
")

echo "JavaScript 输出: $JS_OUTPUT"
echo ""

# Go 版本（需要编译并运行）
echo "编译 Go 版本..."
go build -o xbogus_test ./xbogus.go

if [ -f "./xbogus_test" ]; then
    echo "运行 Go 版本..."
    GO_OUTPUT=$(./xbogus_test)
    echo "Go 输出: $GO_OUTPUT"
    echo ""
    
    if [ "$JS_OUTPUT" == "$GO_OUTPUT" ]; then
        echo "✅ XBogus 输出一致！"
    else
        echo "❌ XBogus 输出不一致！"
        echo "差异:"
        echo "  JS: $JS_OUTPUT"
        echo "  Go: $GO_OUTPUT"
    fi
    
    rm -f ./xbogus_test
else
    echo "❌ Go 编译失败"
fi

echo ""
echo "=== 验证 XGnarly 输出一致性 ==="

QUERY_STRING="device_platform=android&os=android"
BODY="{}"
USER_AGENT="Mozilla/5.0"
ENVCODE=0
VERSION="5.1.1"
TIMESTAMP_MS=1234567890000

echo "输入参数:"
echo "  queryString: $QUERY_STRING"
echo "  body: $BODY"
echo "  userAgent: $USER_AGENT"
echo "  envcode: $ENVCODE"
echo "  version: $VERSION"
echo "  timestampMs: $TIMESTAMP_MS"
echo ""

# JavaScript 版本
echo "运行 JavaScript 版本..."
JS_OUTPUT_XG=$(node -e "
const xgnarly = require('./xgnarly.js');
const result = xgnarly('$QUERY_STRING', '$BODY', '$USER_AGENT', $ENVCODE, '$VERSION', $TIMESTAMP_MS);
console.log(result);
")

echo "JavaScript 输出: $JS_OUTPUT_XG"
echo ""

# Go 版本
echo "编译 Go 版本..."
go build -o xgnarly_test ./xgnarly.go

if [ -f "./xgnarly_test" ]; then
    echo "运行 Go 版本..."
    GO_OUTPUT_XG=$(./xgnarly_test)
    echo "Go 输出: $GO_OUTPUT_XG"
    echo ""
    
    if [ "$JS_OUTPUT_XG" == "$GO_OUTPUT_XG" ]; then
        echo "✅ XGnarly 输出一致！"
    else
        echo "❌ XGnarly 输出不一致！"
        echo "差异:"
        echo "  JS: $JS_OUTPUT_XG"
        echo "  Go: $GO_OUTPUT_XG"
    fi
    
    rm -f ./xgnarly_test
else
    echo "❌ Go 编译失败"
fi

echo ""
echo "=== 验证完成 ==="

