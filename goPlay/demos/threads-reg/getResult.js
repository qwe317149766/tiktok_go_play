const fs = require('fs');

/**
 * 尝试修复并解析 JSON 字符串
 */
function fixAndParseJson(input) {
    if (!input) return { isJson: false, data: input };
    try {
        let str = input.trim();

        // 如果是被包了一层引号的 JSON 字符串
        if (str.startsWith('"') && str.endsWith('"')) {
            str = str.slice(1, -1);
        }

        let result = JSON.parse(str);
        return { isJson: true, data: result };
    } catch (error) {
        let output = input;
        if (output.startsWith('"')) {
            output = output.slice(1);
        }
        if (output.endsWith('"')) {
            output = output.slice(0, -1);
        }
        return { isJson: false, data: output };
    }
}

/**
 * 核心提取方法：根据接口名从清理后的文本中提取参数
 */
function getBloksActionParams(apiName, cleanedText) {
    // 构造匹配模式
    console.log(apiName)
    //将cleanedText写入文件
    const pattern = new RegExp(apiName + '[\\s\\S]*?\\(\\s*f4i\\s*\\(\\s*dkc([\\s\\S]*?regm)', 'g');
    let matches = cleanedText.match(pattern);
    if (!matches) {
        return null;
    }
    // 2️⃣ 构造正则
    const reg = new RegExp(`(\\d+)\\s+(\\d+).{0,10}?${apiName}`);

    // 3️⃣ 匹配
    const match = cleanedText.match(reg);

    // 处理第一个匹配项
    let resContent = matches[0];
    let resArray = resContent.split(apiName + '"');
    let paramsInfo = resArray[1];

    // 调试用：写入提取的部分内容
    // fs.writeFileSync('result.txt', paramsInfo);

    // 1. 提取 Key 数组
    let regexF4i = /\(\s*f4i\s*\(\s*dkc\s*([\s\S]*?)\)/;
    let f4iMatch = paramsInfo.match(regexF4i);
    let keysArray = [];
    if (f4iMatch) {
        let keysStr = f4iMatch[1]
            .replace(/\s/g, ' ')
            .replace(/"/g, ' ')
            .trim();
        keysArray = keysStr.split(/\s+/).filter(Boolean);
    }

    // 2. 提取 Value 数组
    let regexValueDkc = /\(dkc(?:(?!\(dkc)[\s\S])*?\{[\s\S]*?\bregm\b/g;
    let dkcValueMatches = paramsInfo.match(regexValueDkc);

    if (!dkcValueMatches) {
        return null;
    }

    // 清理并分割 Value，处理 (eud 数字) 格式
    let valuesStr = dkcValueMatches[0]
        .replace(/\(dkc/g, '')
        .replace(/\(eud\s*(\d+)\)/gi, '$1')
        .replace(/[\s*](SERVICE_INVALID)/gi, '$1')
        .trim();

    let valuesArray = valuesStr.split(' ').filter(Boolean);
    console.log("valuesArray:", valuesArray)
    // 3. 合并成对象
    let paramsObj = {
        INTERNAL__latency_qpl_marker_id: match[1],
        INTERNAL__latency_qpl_marker_value: match[2]
    };
    for (let i = 0; i < keysArray.length; i++) {
        const key = keysArray[i];
        const rawValue = valuesArray[i] || '';
        let { isJson, data } = fixAndParseJson(rawValue);

        if (isJson) {
            paramsObj[key] = JSON.stringify(data);
        } else {
            paramsObj[key] = data;
        }
    }

    return paramsObj;
}

/**
 * 封装后的最终方法：直接传入接口名和原始文件内容字符串
 * @param {string} apiName 接口名
 * @param {string} rawContent 原始文本内容 (txt/json 文件读取的内容)
 */
function getParamsByApiName(apiName, rawContent) {
    if (!rawContent) return null;

    // 1. 预处理：去掉所有反斜杠 (适应当前正则逻辑)
    let cleanedText = rawContent.replace(/\\/g, '');

    // 2. 调用核心提取逻辑
    return getBloksActionParams(apiName, cleanedText);
}

// --- 导出或本地执行示例 ---

const filePath = 'cleanedText.txt';
if (require.main === module) {
    if (fs.existsSync(filePath)) {
        let content = fs.readFileSync(filePath, 'utf-8');
        //
        console.log("content:", content)
        const targetApi = 'com.bloks.www.bloks.caa.reg.username.async';

        // 使用封装的方法获取结果
        const result = getParamsByApiName(targetApi, content);

        if (result) {
            console.log(`--- [${targetApi}] 提取成功 ---`);
            console.log(JSON.stringify(result, null, 2));
        } else {
            console.log(`未能在响应中找到接口 [${targetApi}] 的参数`);
        }
    } else {
        console.log(`文件不存在: ${filePath}`);
    }
}

// 导出方法供其他模块调用
module.exports = { getParamsByApiName };
