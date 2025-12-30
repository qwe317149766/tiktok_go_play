package headers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"

	"github.com/emmansun/gmsm/sm3"
)

// sub960bc 字节转换函数
func sub960bc(wmz string) string {
	bytes, _ := hex.DecodeString(wmz)
	result := make([]byte, len(bytes))
	for i, b := range bytes {
		tem := ((b & 0xaa) >> 1) | ((b & 0x55) << 1)
		result[i] = ((tem & 0xcc) >> 2) | ((tem & 0x33) << 2)
	}
	return hex.EncodeToString(result)
}

// MakeHex26_1 生成hex26_1签名
func MakeHex26_1(seedEncodeType int, queryString, xSSStub, randd string) string {
	res := ""

	switch seedEncodeType {
	case 1:
		// 对query_string做md5并取前四字节
		partOne := md5HashString(queryString)[:8]

		// 对x-ss-stub做md5并取前四字节
		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		partTwo := md5Hash(xssStubBytes)[:8]

		// 对00000001做md5并取其前四字节
		partThree := md5Hash([]byte{0, 0, 0, 1})[:8]

		res = partOne + partTwo + partThree

	case 2:
		// md5后转换每个字节的大小端序
		md5Query := md5HashString(queryString)[:8]
		partOne := swapNibbles(md5Query)

		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		md5X := md5Hash(xssStubBytes)[:8]
		partTwo := swapNibbles(md5X)

		md5_01 := md5Hash([]byte{0, 0, 0, 1})[:8]
		partThree := swapNibbles(md5_01)

		res = partOne + partTwo + partThree

	case 3:
		// md5后与0x5a异或
		md5Query := md5HashString(queryString)[:8]
		partOne := xorHexWith5a(md5Query)

		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		md5X := md5Hash(xssStubBytes)[:8]
		partTwo := xorHexWith5a(md5X)

		md5_01 := md5Hash([]byte{0, 0, 0, 1})[:8]
		partThree := xorHexWith5a(md5_01)

		res = partOne + partTwo + partThree

	case 4:
		// md5后进行sub960bc处理
		md5Query := md5HashString(queryString)[:8]
		after960bc := sub960bc(md5Query)
		partOne := swapNibbles(after960bc)

		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		md5X := md5Hash(xssStubBytes)[:8]
		after960bcX := sub960bc(md5X)
		partTwo := swapNibbles(after960bcX)

		md5_01 := md5Hash([]byte{0, 0, 0, 1})[:8]
		after960bc01 := sub960bc(md5_01)
		partThree := swapNibbles(after960bc01)

		res = partOne + partTwo + partThree

	case 5:
		// sm3取最后4字节
		hash1 := sm3.Sum([]byte(queryString))
		partOne := hex.EncodeToString(hash1[:])[56:64]

		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		hash2 := sm3.Sum(xssStubBytes)
		partTwo := hex.EncodeToString(hash2[:])[56:64]

		hash3 := sm3.Sum([]byte{0, 0, 0, 1})
		partThree := hex.EncodeToString(hash3[:])[56:64]

		res = partOne + partTwo + partThree

	case 6:
		// AES-OFB加密
		randdBytes, _ := hex.DecodeString(randd)
		md5Randd := md5Hash(randdBytes)

		key := []byte(md5Randd[:16])
		iv := []byte(md5Randd[16:])

		// query_string
		block1, _ := aes.NewCipher(key)
		stream1 := cipher.NewOFB(block1, iv)
		queryPadded := pkcs7Pad([]byte(queryString), 16)
		encrypted1 := make([]byte, len(queryPadded))
		stream1.XORKeyStream(encrypted1, queryPadded)
		partOne := hex.EncodeToString(encrypted1)[len(encrypted1)*2-8:]

		// x-ss-stub
		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		block2, _ := aes.NewCipher(key)
		stream2 := cipher.NewOFB(block2, iv)
		xssPadded := pkcs7Pad(xssStubBytes, 16)
		encrypted2 := make([]byte, len(xssPadded))
		stream2.XORKeyStream(encrypted2, xssPadded)
		partTwo := hex.EncodeToString(encrypted2)[len(encrypted2)*2-8:]

		// 00000001
		block3, _ := aes.NewCipher(key)
		stream3 := cipher.NewOFB(block3, iv)
		padded01 := pkcs7Pad([]byte{0, 0, 0, 1}, 16)
		encrypted3 := make([]byte, len(padded01))
		stream3.XORKeyStream(encrypted3, padded01)
		partThree := hex.EncodeToString(encrypted3)[len(encrypted3)*2-8:]

		res = partOne + partTwo + partThree

	case 7:
		// sha256 + md5 + rc4
		sha256Query := sha256.Sum256([]byte(queryString))
		partOne := xorHexWith5a(hex.EncodeToString(sha256Query[:])[:8])

		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		md5X := md5Hash(xssStubBytes)
		partTwo := md5X[24:32]

		randdBytes, _ := hex.DecodeString(randd)
		md5Randd := md5Hash(randdBytes)
		rc4Key := []byte(md5Randd)
		cipher, _ := rc4.NewCipher(rc4Key)
		ciphertext := make([]byte, 4)
		cipher.XORKeyStream(ciphertext, []byte{0, 0, 0, 1})
		after960bcRc4 := sub960bc(hex.EncodeToString(ciphertext))
		partThree := swapNibbles(after960bcRc4)

		res = partOne + partTwo + partThree

	case 8:
		// sha1 + crc32 + sha256
		sha1Query := sha1.Sum([]byte(queryString))
		partOne := xorHexWith5a(hex.EncodeToString(sha1Query[:])[:8])

		xssStubBytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		if xSSStub != "" {
			xssStubBytes, _ = hex.DecodeString(xSSStub)
		}
		crcVal := crc32.ChecksumIEEE(xssStubBytes)
		crcHex := toFixedHex(intToHexStr(uint64(crcVal)), 8)
		after960bcCrc := sub960bc(crcHex)
		partTwo := swapNibbles(after960bcCrc)

		sha256_01 := sha256.Sum256([]byte{0, 0, 0, 1})
		partThree := swapNibbles(hex.EncodeToString(sha256_01[:])[:8])

		res = partOne + partTwo + partThree
	}

	// 最后与randd异或
	ans := ""
	randdBytes, _ := hex.DecodeString(randd)
	randdReversed := reverseBytes(randdBytes)
	randddHex := hex.EncodeToString(randdReversed)

	resBytes, _ := hex.DecodeString(res)
	for i := 0; i < len(resBytes); i++ {
		randdByteIdx := (i % 4) * 2
		randdByte, _ := hex.DecodeString(randddHex[randdByteIdx : randdByteIdx+2])
		ans += toFixedHex(intToHexStr(uint64(resBytes[i]^randdByte[0])), 2)
	}

	// 反转结果
	anss := ""
	for i := len(ans)/2 - 1; i >= 0; i-- {
		anss += ans[i*2 : i*2+2]
	}

	return anss
}

// swapNibbles 交换每个字节的两个十六进制字符
func swapNibbles(hexStr string) string {
	result := ""
	for i := 0; i < len(hexStr); i += 2 {
		if i+1 < len(hexStr) {
			result += string(hexStr[i+1]) + string(hexStr[i])
		}
	}
	return result
}

// xorHexWith5a 将每个字节与0x5a异或
func xorHexWith5a(hexStr string) string {
	bytes, _ := hex.DecodeString(hexStr)
	result := make([]byte, len(bytes))
	for i, b := range bytes {
		result[i] = b ^ 0x5a
	}
	return hex.EncodeToString(result)
}

// reverseBytes 反转字节数组
func reverseBytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i, b := range data {
		result[len(data)-1-i] = b
	}
	return result
}
func main() {
	res := MakeHex26_1(7, "os=android&_rticket=1765616610317&is_pad=0&last_install_time=1764813628&host_abi=arm64-v8a&ts=1765616610&ab_version=42.4.3&ac=wifi&ac2=wifi&aid=1233&app_language=en&app_name=musical_ly&app_type=normal&build_number=42.4.3&carrier_region=US&carrier_region_v2=310&channel=googleplay&current_region=US&device_brand=google&device_id=7583179433013052983&device_platform=android&device_type=Pixel%206&dpi=420&iid=7583179514218350349&language=en&locale=en&manifest_version_code=2024204030&mcc_mnc=310004&op_region=US&os_api=35&os_version=15&region=US&residence=US&resolution=1080*2209&ssmix=a&sys_region=US&timezone_name=America%2FNew_York&timezone_offset=-18000&uoo=0&update_version_code=2024204030&version_code=420403&version_name=42.4.3", "143a194474f34a8adbb55dac550b5202", "6AB1329C")
	fmt.Println("res===>", res)
}
