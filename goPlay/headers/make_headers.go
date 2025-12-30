package headers

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"

	"github.com/emmansun/gmsm/sm3"

	"tt_code/tt_protobuf"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// HeadersResult 返回生成的headers结果
type HeadersResult struct {
	XSSStub  string
	XKhronos string
	XArgus   string
	XLadon   string
	XGorgon  string
}

// MakeHeaders 生成所有需要的headers签名 - 完整实现与Python一致
func MakeHeaders(
	deviceID string,
	createTime int64,
	signCount int,
	reportCount int,
	settingCount int,
	appLaunchTime int64,
	secDeviceToken string,
	phoneInfo string,
	seed string,
	seedEncodeType int,
	seedEndcodeHex string,
	algorithmData1 string,
	hex32 string,
	queryString string,
	postData string,
	appVersion string,
	sdkVersionStr string,
	sdkVersion int,
	callType int,
	appVersionConstant int,
) *HeadersResult {
	// 设置默认值
	if appVersion == "" {
		appVersion = "42.4.3"
	}
	if sdkVersionStr == "" {
		sdkVersionStr = "v05.02.02-ov-android"
	}
	if sdkVersion == 0 {
		sdkVersion = 0x05020220
	}
	if callType == 0 {
		callType = 738
	}
	if appVersionConstant == 0 {
		appVersionConstant = 0xC40A800
	}

	// 计算x-ss-stub
	var xSSStub string
	if postData != "" {
		postDataBytes, _ := hex.DecodeString(postData)
		hash := md5.Sum(postDataBytes)
		xSSStub = hex.EncodeToString(hash[:])
	} else {
		xSSStub = "00000000000000000000000000000000"
	}

	// 计算p13 (对x-ss-stub做sm3)
	xSSStubBytes, _ := hex.DecodeString(xSSStub)
	p13Hash := sm3.Sum(xSSStubBytes)
	p13 := hex.EncodeToString(p13Hash[:])
	bodyHash := p13[:12]

	// 计算p14 (对query_string做sm3后取前6字节)
	p14Hash := sm3.Sum([]byte(queryString))
	p14 := hex.EncodeToString(p14Hash[:])
	queryHash := p14[:12]

	// 计算pskCalHash (对query_string.encode("utf8").hex() + x_ss_stub + "30" 做sm3)
	// Python: sm3.sm3_hash(func.bytes_to_list((bytes.fromhex(query_string.encode("utf8").hex()+x_ss_stub+"30"))))
	queryStringHex := hex.EncodeToString([]byte(queryString))
	combinedHex := queryStringHex + xSSStub + "30"
	combinedBytes, _ := hex.DecodeString(combinedHex)
	pskCalHashBytes := sm3.Sum(combinedBytes)
	pskCalHash := hex.EncodeToString(pskCalHashBytes[:])

	// 生成rand_26 (与Python保持一致: rand_26 = "6A51C28C")
	rand26 := "6A51C28C"
	// rand26 := fmt.Sprintf("%08X", rand.Uint32()) // 如果需要随机，取消注释

	// 如果seedEncodeType不为空，计算seed相关的值 (Python: if seed_encode_type !="")
	// 注意：Python 中 seed_encode_type 是 int 类型，但判断是 !=""，可能是类型不一致
	// 在 Go 中，如果 seedEncodeType != 0 就执行（0 表示未设置）
	if seedEncodeType != 0 {
		seedEndcodeHex = MakeHex26_1(seedEncodeType, queryString, xSSStub, rand26)
		algorithmData1 = MakeHex26_2(p14, p13)
		hex32 = "" // 暂时固定为空
	}

	// 转大写x_ss_stub（与Python一致：先转大写再用于后续计算）
	xSSStub = toUpperHex(xSSStub)

	// 生成argus protobuf
	xArgusProtobuf := tt_protobuf.MakeOneArgusPb(
		deviceID, appVersion, sdkVersionStr, sdkVersion, createTime,
		bodyHash, queryHash, signCount, reportCount, settingCount,
		appLaunchTime, secDeviceToken, "", pskCalHash, callType,
		phoneInfo, appVersionConstant, seed, seedEncodeType, seedEndcodeHex,
		algorithmData1, hex32, rand26,
	)
	// 生成各个签名
	xArgus := MakeArgus(xArgusProtobuf, queryHash)
	xLadon := MakeLadon(fmt.Sprintf("%d", createTime), "31323333")
	// Python: gorgon.make_gorgon(khronos=str(x_khronos), ...) - khronos 是十进制字符串
	// 注意：Python 传入的 x_ss_stub 是大写的
	xGorgon := MakeGorgon(fmt.Sprintf("%d", createTime), queryString, xSSStub, "0000000020020205")

	return &HeadersResult{
		XSSStub:  xSSStub,
		XKhronos: fmt.Sprintf("%d", createTime),
		XArgus:   xArgus,
		XLadon:   xLadon,
		XGorgon:  xGorgon,
	}
}

// toUpperHex 将十六进制字符串转换为大写
func toUpperHex(s string) string {
	result := ""
	for _, c := range s {
		if c >= 'a' && c <= 'f' {
			result += string(c - 32)
		} else {
			result += string(c)
		}
	}
	return result
}
