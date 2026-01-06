package main

import (
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
	// const reg = new RegExp(`(\\d+)\\s+(\\d+).{0,10}?${apiName}`);
	regMarker := regexp.MustCompile(`(\d+)\s+(\d+).{0,10}?` + regexp.QuoteMeta(apiName))
	markerMatch := regMarker.FindStringSubmatch(cleanedText)

	// pattern = new RegExp(apiName + '[\\s\\S]*?\\(\\s*f4i\\s*\\(\\s*dkc([\\s\\S]*?regm)', 'g');
	pattern := regexp.MustCompile(regexp.QuoteMeta(apiName) + `[\s\S]*?\(\s*f4i\s*\(\s*dkc([\s\S]*?regm)`)
	matches := pattern.FindStringSubmatch(cleanedText)
	if len(matches) == 0 {
		return nil
	}

	// 处理第一个匹配项
	resContent := matches[0]
	// resArray = resContent.split(apiName + '"')
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
