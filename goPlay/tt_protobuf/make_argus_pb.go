package tt_protobuf

import (
	"encoding/hex"
	"fmt"
)

// MakeOneArgusPb 生成Argus protobuf消息 - 完整实现
func MakeOneArgusPb(
	deviceID string,
	appVersion string,
	sdkVersionStr string,
	sdkVersion int,
	createTime int64,
	bodyHash string,
	queryHash string,
	signCount int,
	reportCount int,
	settingCount int,
	appLaunchTime int64,
	secDeviceToken string,
	pskHash string,
	pskCalHash string,
	callType int,
	phoneInfo string,
	appVersionConstant int,
	seed string,
	seedEncodeType int,
	seedEncodeHex string,
	algorithmData1 string,
	hex32 string,
	rand26 string,
) string {
	e := NewProtobufEncoder()

	// field 1: magic = 0x20200929 << 1
	e.WriteInt32(1, int32(0x20200929<<1))

	// field 2: version = 2
	e.WriteInt32(2, 2)

	// field 3: rand = int(rand_26,16) << 1
	rand26Val, _ := parseHexInt64(rand26)
	e.WriteInt64(3, rand26Val<<1)

	// field 4: msAppID = "1233"
	e.WriteString(4, "1233")

	// field 5: deviceID
	if deviceID != "" {
		e.WriteString(5, deviceID)
	}

	// field 6: licenseID = "2142840551"
	e.WriteString(6, "2142840551")

	// field 7: appVersion
	e.WriteString(7, appVersion)

	// field 8: sdkVersionStr
	e.WriteString(8, sdkVersionStr)

	// field 9: sdkVersion << 1
	e.WriteInt32(9, int32(sdkVersion<<1))

	// field 10: envCode = bytes.fromhex("0000000000000000")
	e.WriteBytes(10, []byte{0, 0, 0, 0, 0, 0, 0, 0})

	// field 12: createTime << 1 (注意：proto中field 11是platform，Python代码中没有设置)
	e.WriteInt64(12, createTime<<1)

	// field 13: bodyHash
	bodyHashBytes, _ := hex.DecodeString(bodyHash)
	e.WriteBytes(13, bodyHashBytes)

	// field 14: queryHash
	queryHashBytes, _ := hex.DecodeString(queryHash)
	e.WriteBytes(14, queryHashBytes)

	// field 15: actionRecord (嵌套消息)
	actionRecord := encodeActionRecord(signCount, reportCount, settingCount, appLaunchTime, seedEncodeType)
	e.WriteMessage(15, actionRecord)

	// field 16: secDeviceToken
	e.WriteString(16, secDeviceToken)

	// field 17: isAppLicense = createTime << 1
	e.WriteInt64(17, createTime<<1)

	// field 18: pskHash (Python代码中pskHash被设置为空字符串，所以不会写入)
	// Python: pskHash = "" 然后 if pskHash!="": ... 所以实际上不会写入
	// 因此这里也不写入

	// field 19: pskCalHash (如果不为空)
	if pskCalHash != "" {
		pskCalHashBytes, _ := hex.DecodeString(pskCalHash)
		e.WriteBytes(19, pskCalHashBytes)
	}

	// field 20: pskVersion = "0"
	e.WriteString(20, "0")

	// field 21: callType = 738
	e.WriteInt32(21, 738)

	// field 23: channelInfo (嵌套消息，注意：proto中field 22不存在，所以是23)
	channelInfo := encodeChannelInfo(phoneInfo, appVersionConstant)
	e.WriteMessage(23, channelInfo)

	// field 24: seed
	if seed != "" {
		e.WriteString(24, seed)
	}

	// field 25: extType = 10 (固定值，不再是随机)
	e.WriteInt32(25, 10)

	// field 26: extraInfo (如果seedEncodeType不为空，注意：proto中field 26是repeated)
	if seedEncodeType != 0 {
		// 第一个extraInfo: algorithm = seedEncodeType<<1, algorithmData = seedEncodeHex
		extraInfo1 := encodeExtraInfo(seedEncodeType<<1, seedEncodeHex)
		e.WriteMessage(26, extraInfo1)

		// 第二个extraInfo: algorithm = 2016, algorithmData = algorithmData1
		extraInfo2 := encodeExtraInfo(2016, algorithmData1)
		e.WriteMessage(26, extraInfo2)
	}
	e.WriteInt64(27, createTime<<1)
	// field 28: unknown28 = 1006 (注意：proto中field 27不存在，所以是28)
	e.WriteInt32(28, 1006)

	// field 29: unknown29 = 516112
	e.WriteInt32(29, 516112)

	// field 30: unknown30 = 6
	e.WriteInt32(30, 6)

	// field 31: unknown31 - 复杂计算（更新后的逻辑）
	// Python: tem = f'{signCount&0xff:02x}'
	// Python: aaa = int(f"{0x82 ^ 0x38 ^ int(tem, 16) & 0xff:02x}82{tem}38", 16) ^ create_time ^ int(rand_26, 16)
	// Python: if aaa & 0x80000000: aaa = (abs(aaa - 0x100000000) << 1) - 1
	// Python: else: aaa <<= 1
	tem := fmt.Sprintf("%02x", signCount&0xff) // f'{signCount&0xff:02x}'
	temInt, _ := parseHexInt64(tem)
	xorVal := (0x82 ^ 0x38 ^ int(temInt)) & 0xff
	xorHex := fmt.Sprintf("%02x", xorVal)                    // f"{0x82 ^ 0x38 ^ int(tem, 16) & 0xff:02x}"
	combinedHex := xorHex + "82" + tem + "38"                // f"{0x82 ^ 0x38 ^ int(tem, 16) & 0xff:02x}82{tem}38"
	combinedInt, _ := parseHexInt64(combinedHex)
	aaa := combinedInt ^ createTime ^ rand26Val
	// 处理符号位（将 aaa 视为32位有符号整数）
	aaa32 := int32(aaa) // 转换为32位整数以检查符号位
	const signBitMask = uint32(0x80000000)
	if uint32(aaa32)&signBitMask != 0 {
		// 如果高位为 1，则是负数，需要减去 2^32 然后取绝对值
		// abs(aaa - 0x100000000) 等价于 0x100000000 - aaa
		// 0x100000000 = 4294967296，使用 uint64 计算
		aaa = int64(uint64(0x100000000) - uint64(uint32(aaa32)))
		aaa = (aaa << 1) - 1
	} else {
		aaa <<= 1 // 否则就是正数
	}
	e.WriteInt64(31, aaa)
	// e.WriteInt64(31, 772383712)
	// field 32: unknown32 (如果hex32不为空)
	if hex32 != "" {
		hex32Bytes, _ := hex.DecodeString(hex32)
		e.WriteBytes(32, hex32Bytes)
	}

	// field 33: unknown33 = 4
	e.WriteInt32(33, 4)

	return hex.EncodeToString(e.Bytes())
}

// encodeActionRecord 编码ActionRecord消息
func encodeActionRecord(signCount, reportCount, settingCount int, appLaunchTime int64, seedEncodeType int) []byte {
	e := NewProtobufEncoder()

	// signCount << 1
	e.WriteInt32(1, int32(signCount<<1))

	// reportCount = 4 (固定值)
	e.WriteInt32(2, 4)

	// reportFailCount = 4 (固定值)
	e.WriteInt32(5, 4)

	// actionIncremental = 6 (固定值，不再是随机)
	e.WriteInt32(6, 6)

	// appLaunchTime << 1
	e.WriteInt64(7, appLaunchTime<<1)

	// seed_type << 1 (如果有)
	if seedEncodeType != 0 {
		e.WriteInt32(8, int32(seedEncodeType<<1))
	}
	return e.Bytes()
}

// encodeChannelInfo 编码ChannelInfo消息
func encodeChannelInfo(phoneInfo string, appVersionConstant int) []byte {
	e := NewProtobufEncoder()

	// phoneInfo
	e.WriteString(1, phoneInfo)

	// metasecConstant = 22
	e.WriteInt32(2, 22)

	// channel = "googleplay"
	e.WriteString(3, "samsung_store")

	// appVersionConstant << 1
	e.WriteInt32(4, int32(appVersionConstant<<1))

	return e.Bytes()
}

// encodeExtraInfo 编码ExtraInfo消息
func encodeExtraInfo(algorithm int, algorithmData string) []byte {
	e := NewProtobufEncoder()

	// algorithm
	e.WriteInt32(1, int32(algorithm))

	// algorithmData
	if algorithmData != "" {
		dataBytes, _ := hex.DecodeString(algorithmData)
		e.WriteBytes(2, dataBytes)
	}

	return e.Bytes()
}

// parseHexInt64 解析16进制字符串为int64
func parseHexInt64(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	var result int64
	for _, c := range s {
		result <<= 4
		if c >= '0' && c <= '9' {
			result |= int64(c - '0')
		} else if c >= 'a' && c <= 'f' {
			result |= int64(c - 'a' + 10)
		} else if c >= 'A' && c <= 'F' {
			result |= int64(c - 'A' + 10)
		} else {
			// 无效字符，返回错误或忽略
			return 0, fmt.Errorf("invalid hex character: %c", c)
		}
	}
	return result, nil
}
