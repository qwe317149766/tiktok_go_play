package headers

import (
	"encoding/hex"
	"fmt"
)

// makeLadonData1Of1 ladon数据处理第一步
func makeLadonData1Of1(aa, a0 string, i int) (string, string) {
	if i == 0 {
		return aa, a0
	}

	aaVal, _ := parseHexUint64(aa)
	a0Val, _ := parseHexUint64(a0)

	tem := (ror64(aaVal, 8) + a0Val) & 0xFFFFFFFFFFFFFFFF
	newAa := tem ^ uint64(i-1)
	newA0 := ror64(a0Val, 61) ^ newAa

	return intToHexStr(newAa), intToHexStr(newA0)
}

// makeLadonData2Of1 ladon数据处理第二步
func makeLadonData2Of1(b0, b1 string) string {
	b0Val, _ := parseHexUint64(b0)
	b1Val, _ := parseHexUint64(b1)
	tem := (b0Val + ror64(b1Val, 8)) & 0xFFFFFFFFFFFFFFFF
	return intToHexStr(tem)
}

// makeLadonData1Of2 ladon数据处理第三步
func makeLadonData1Of2(b0 string) string {
	b0Val, _ := parseHexUint64(b0)
	return intToHexStr(ror64(b0Val, 0x3d))
}

// makeLadonData ladon数据主处理函数
func makeLadonData(md5Res, timeSign string) string {
	res := ""

	a0 := bigEndianToLittle(md5Res[:16])
	a1 := bigEndianToLittle(md5Res[16:32])
	a2 := bigEndianToLittle(md5Res[32:48])
	a3 := bigEndianToLittle(md5Res[48:64])
	b0 := bigEndianToLittle(timeSign[:16])
	b1 := bigEndianToLittle(timeSign[16:32])
	b2 := bigEndianToLittle(timeSign[32:48])
	b3 := bigEndianToLittle(timeSign[48:64])

	aa := []string{a1, a2, a3}

	for i := 0; i < 34; i++ {
		if i != 0 {
			cs := (i%3 - 1)
			if i%3 == 0 {
				cs = 2
			}
			aa[cs], a0 = makeLadonData1Of1(aa[cs], a0, i)
		}
		tem := makeLadonData2Of1(b0, b1)
		temVal, _ := parseHexUint64(tem)
		a0Val, _ := parseHexUint64(a0)
		b1 = toFixedHex(intToHexStr(a0Val^temVal), 16)
		b0 = makeLadonData1Of2(b0)
		b0Val, _ := parseHexUint64(b0)
		b1Val, _ := parseHexUint64(b1)
		b0 = toFixedHex(intToHexStr(b0Val^b1Val), 16)
	}

	res += littleEndianToBig(b0) + littleEndianToBig(b1)

	// 重置并进行第二轮
	a0 = bigEndianToLittle(md5Res[:16])
	aa = []string{a1, a2, a3}

	for i := 0; i < 34; i++ {
		if i != 0 {
			cs := (i%3 - 1)
			if i%3 == 0 {
				cs = 2
			}
			aa[cs], a0 = makeLadonData1Of1(aa[cs], a0, i)
		}
		tem := makeLadonData2Of1(b2, b3)
		temVal, _ := parseHexUint64(tem)
		a0Val, _ := parseHexUint64(a0)
		b3 = toFixedHex(intToHexStr(a0Val^temVal), 16)
		b2 = makeLadonData1Of2(b2)
		b2Val, _ := parseHexUint64(b2)
		b3Val, _ := parseHexUint64(b3)
		b2 = toFixedHex(intToHexStr(b2Val^b3Val), 16)
	}

	res += littleEndianToBig(b2) + littleEndianToBig(b3)

	return res
}

// MakeLadon 生成X-Ladon签名
func MakeLadon(khronos, aid string) string {
	if aid == "" {
		aid = "31323333" // "1233" 的hex编码
	}

	theFirstFour := makeRand()
	// Python: md5_res=md5(bytearray.fromhex(the_first_four+aid)).encode().hex()
	// 先拼接 hex 字符串，转成字节，做 MD5，得到 hex 字符串，再 encode().hex()
	combinedHex := theFirstFour + aid
	combinedBytes, _ := hex.DecodeString(combinedHex)
	md5ResStr := md5Hash(combinedBytes) // MD5 返回 hex 字符串
	md5Res := hex.EncodeToString([]byte(md5ResStr)) // encode().hex()

	// 时间戳+lc_id+appID、最后填充至32字节
	timeSignStr := khronos + "-2142840551-1233"
	timeSign := hex.EncodeToString([]byte(timeSignStr)) + "060606060606"

	theLastThirtyTwo := makeLadonData(md5Res, timeSign)
	forBase64, _ := hex.DecodeString(theFirstFour + theLastThirtyTwo)
	ladon := toBase64(forBase64)

	return ladon
}

// khronosToHex 将时间戳转换为16进制字符串
func khronosToHex(khronos string) string {
	var ts int64
	fmt.Sscanf(khronos, "%d", &ts)
	return fmt.Sprintf("%x", ts)
}
