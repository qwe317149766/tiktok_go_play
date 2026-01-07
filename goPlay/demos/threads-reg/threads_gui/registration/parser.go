package registration

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
)

// FixAndParseJson mimics the behavior of the JavaScript version's fixAndParseJson
func FixAndParseJson(input string) string {
	if input == "" {
		return input
	}
	str := strings.TrimSpace(input)
	originalStr := str

	// 如果是被包了一层引号的 JSON 字符串
	if strings.HasPrefix(str, "\"") && strings.HasSuffix(str, "\"") {
		str = str[1 : len(str)-1]
	}

	var js interface{}
	if json.Unmarshal([]byte(str), &js) == nil {
		// 如果是 JSON，返回其字符串形式（相当于 JSON.stringify）
		b, _ := json.Marshal(js)
		return string(b)
	}

	// Catch block logic
	output := originalStr
	if strings.HasPrefix(output, "\"") {
		output = output[1:]
	}
	if strings.HasSuffix(output, "\"") {
		output = output[:len(output)-1]
	}
	return output
}

// GetParamsByApiName extracts parameters from raw bloks response content
// Directly corresponds to the JS getParamsByApiName
func GetParamsByApiName(apiName string, rawContent string) map[string]string {
	if rawContent == "" {
		return nil
	}

	// 1. 预处理：去掉所有反斜杠 (适应当前正则逻辑)
	cleanedText := strings.ReplaceAll(rawContent, "\\", "")

	// 2. 调用核心提取逻辑
	return GetBloksActionParams(apiName, cleanedText)
}

// GetBloksActionParams core extraction logic matching getBloksActionParams in JS
func GetBloksActionParams(apiName string, cleanedText string) map[string]string {
	// 2️⃣ 构造正则 (INTERNAL markers)
	regMarker := regexp.MustCompile(`(\d+)\s+(\d+).{0,10}?` + regexp.QuoteMeta(apiName))
	markerMatch := regMarker.FindStringSubmatch(cleanedText)

	pattern := regexp.MustCompile(regexp.QuoteMeta(apiName) + `[\s\S]*?\(\s*f4i\s*\(\s*dkc([\s\S]*?regm)`)
	matches := pattern.FindStringSubmatch(cleanedText)
	if len(matches) == 0 {
		return nil
	}

	// 处理第一个匹配项
	resContent := matches[0]
	resArray := strings.Split(resContent, apiName+"\"")
	if len(resArray) < 2 {
		return nil
	}
	paramsInfo := resArray[1]

	// 1. 提取 Key 数组
	regexF4i := regexp.MustCompile(`\(\s*f4i\s*\(\s*dkc\s*([\s\S]*?)\)`)
	f4iMatch := regexF4i.FindStringSubmatch(paramsInfo)
	var keysArray []string
	if len(f4iMatch) > 1 {
		keysStr := f4iMatch[1]
		keysStr = strings.ReplaceAll(keysStr, "\n", " ")
		keysStr = strings.ReplaceAll(keysStr, "\r", " ")
		keysStr = strings.ReplaceAll(keysStr, "\t", " ")
		keysStr = strings.ReplaceAll(keysStr, "\"", " ")
		keysStr = strings.TrimSpace(keysStr)
		keysArray = strings.Fields(keysStr)
	}

	// 2. 提取 Value 数组
	var valuesArray []string
	reBlockContent := regexp.MustCompile(`\{[\s\S]*?\bregm\b`)
	blockLoc := reBlockContent.FindStringIndex(paramsInfo)
	if blockLoc != nil {
		prefix := paramsInfo[:blockLoc[0]]
		dkcIdx := strings.LastIndex(prefix, "(dkc")
		if dkcIdx != -1 {
			valuesStr := paramsInfo[dkcIdx:blockLoc[1]]

			// 清理并分割 Value，处理 (eud 数字) 格式
			valuesStr = strings.ReplaceAll(valuesStr, "(dkc", "")

			reEud := regexp.MustCompile(`(?i)\(eud\s*(\d+)\)`)
			valuesStr = reEud.ReplaceAllString(valuesStr, "$1")

			reServiceInvalid := regexp.MustCompile(`(?i)[\s\*](SERVICE_INVALID)`)
			valuesStr = reServiceInvalid.ReplaceAllString(valuesStr, "$1")

			valuesStr = strings.TrimSpace(valuesStr)
			valuesArray = strings.Fields(valuesStr)
		}
	}

	if len(valuesArray) == 0 {
		return nil
	}

	// 3. 合并成对象
	paramsObj := make(map[string]string)
	// Set internal markers if found
	if len(markerMatch) > 2 {
		paramsObj["INTERNAL__latency_qpl_marker_id"] = markerMatch[1]
		paramsObj["INTERNAL__latency_qpl_marker_value"] = markerMatch[2]
	}

	for i, key := range keysArray {
		rawValue := ""
		if i < len(valuesArray) {
			rawValue = valuesArray[i]
		}
		paramsObj[key] = FixAndParseJson(rawValue)
	}

	return paramsObj
}

// ExtractTokenAndUsername attempts to pull username, registration token, pkid, sessionid, nonce, and fbid_v2 from a JSON response
func ExtractTokenAndUsername(data string) (string, string, string, string, string, string) {
	// 1. Username pattern
	userRe := regexp.MustCompile(`(?i)username[\\"]+:[\\"\s]*[\\"]+([^"\\]+)`)
	userMatch := userRe.FindStringSubmatch(data)
	username := ""
	if len(userMatch) > 1 {
		username = userMatch[1]
	}

	// 2. Authorization Token pattern
	authRe := regexp.MustCompile(`(?i)IG-Set-Authorization[\\"]+:[\\"\s]+Bearer\s+IGT:2:([^"\\]+)`)
	authMatch := authRe.FindStringSubmatch(data)
	token := ""
	fullAuth := ""
	pkid := ""
	sessionid := ""

	if len(authMatch) > 1 {
		token = authMatch[1]
		fullAuth = "Bearer IGT:2:" + token

		// Decode the base64 token
		decoded, err := base64.StdEncoding.DecodeString(token)
		if err == nil {
			var authMap map[string]any
			if err := json.Unmarshal(decoded, &authMap); err == nil {
				if sid, ok := authMap["sessionid"].(string); ok {
					sessionid = sid
				}
				if uid, ok := authMap["ds_user_id"].(string); ok {
					pkid = uid
				}
			}
		}
	}

	// 3. Fallback PKID pattern
	if pkid == "" {
		pkRe := regexp.MustCompile(`(?i)pk[\\"]+:[\\"\s]*[\\"]*(\d+)`)
		pkMatch := pkRe.FindStringSubmatch(data)
		if len(pkMatch) > 1 {
			pkid = pkMatch[1]
		}
	}

	// 4. Nonce pattern
	nonceRe := regexp.MustCompile(`(?i)(?:session_flush_nonce|partially_created_account_nonce)[\\"]+:[\\"\s]*[\\"]+([^"\\]+)`)
	nonceMatch := nonceRe.FindStringSubmatch(data)
	nonce := ""
	if len(nonceMatch) > 1 {
		nonce = nonceMatch[1]
	}

	// 5. fbid_v2 pattern
	fbidRe := regexp.MustCompile(`fbid_v2[\\"]+:(\d+)`)
	fbidMatch := fbidRe.FindStringSubmatch(data)
	fbidV2 := ""
	if len(fbidMatch) > 1 {
		fbidV2 = fbidMatch[1]
	}

	return username, fullAuth, pkid, sessionid, nonce, fbidV2
}
