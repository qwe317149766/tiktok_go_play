package headers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"

	"github.com/emmansun/gmsm/sm3"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// intToHexStr 将数字转换为16进制字符串
func intToHexStr(num uint64) string {
	return fmt.Sprintf("%x", num)
}

// ror64 8字节循环右移
func ror64(value uint64, shift int) uint64 {
	shift = shift % 64
	return (value >> uint(shift)) | (value << uint(64-shift))
}

// lsl64 8字节左移
func lsl64(value uint64, shift int) uint64 {
	return (value << uint(shift)) & 0xFFFFFFFFFFFFFFFF
}

// lsr64 8字节右移
func lsr64(value uint64, shift int) uint64 {
	return value >> uint(shift)
}

// bigEndianToLittle 将16进制字符串由大端序转换为小端序
func bigEndianToLittle(hexStr string) string {
	bytes, _ := hex.DecodeString(hexStr)
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return hex.EncodeToString(bytes)
}

// littleEndianToBig 将16进制字符串由小端序转换为大端序
func littleEndianToBig(hexStr string) string {
	bytes, _ := hex.DecodeString(hexStr)
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return hex.EncodeToString(bytes)
}

// toBase64 将字节数组转换为base64字符串
func toBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// fromBase64 从base64解码并返回16进制字符串
func fromBase64(data string) string {
	decoded, _ := base64.StdEncoding.DecodeString(data)
	return hex.EncodeToString(decoded)
}

// md5Hash 计算MD5哈希
func md5Hash(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

// md5HashString 计算字符串的MD5哈希
func md5HashString(data string) string {
	return md5Hash([]byte(data))
}

// sm3Hash 计算SM3哈希
func sm3Hash(data []byte) string {
	hash := sm3.Sum(data)
	return hex.EncodeToString(hash[:])
}

// makeRand 生成一个四字节16进制随机数
func makeRand() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// toFixedHex 将16进制字符串填充到指定长度
func toFixedHex(value string, length int) string {
	for len(value) < length {
		value = "0" + value
	}
	return value
}

// MakeArgusRes1Aes3AndKey 生成后续n*72轮加密运算所需要的key
func MakeArgusRes1Aes3AndKey(signKey string) (string, string, string) {
	part1_3 := fromBase64(signKey)
	part2 := toFixedHex(makeRand(), 8)
	res1, res3 := part2[:4], part2[4:]

	forSm3 := part1_3 + part2 + part1_3
	forSm3Bytes, _ := hex.DecodeString(forSm3)
	sm3Res := sm3Hash(forSm3Bytes)

	res := bigEndianToLittle(sm3Res[:16]) + bigEndianToLittle(sm3Res[16:32]) +
		bigEndianToLittle(sm3Res[32:48]) + bigEndianToLittle(sm3Res[48:64])

	return res1, res3, res
}

// makeArgusEorDataKeyList 负责生成第五轮及其之后的key
func makeArgusEorDataKeyList(k []string) []string {
	s1 := uint64(0xf2101d113b815d60)
	s2 := uint64(0x0defe2eec47ea29f)
	s3 := uint64(0x8db0dcd8e81a9b3e)
	s4 := uint64(0x724f232717e564c1)
	s5 := uint64(0xc236b3c5fb929874)

	for i := 4; i < 75; i++ {
		k4, _ := parseHexUint64(k[i-1])
		k2, _ := parseHexUint64(k[i-3])
		k1, _ := parseHexUint64(k[i-4])

		tem1 := s1 & ror64(k4, 3)
		tem2 := s2 & lsr64(k4, 3)
		tem3 := tem1 | tem2
		tem4 := k2 ^ tem3
		tem5 := uint64(0xe000000000000000) ^ tem4

		tem6 := s3 & ror64(tem5, 1)
		tem7 := s4 & lsr64(tem5, 1)

		shiftVal := i - 4
		if shiftVal > 0x3d {
			shiftVal = (shiftVal % 0x3d) - 1
		}
		tem8 := lsr64(s5, shiftVal)

		tem9 := k1 ^ tem5
		tem10 := tem6 | tem7
		num := tem8 & 1
		tem11 := tem9 ^ tem10
		tem12 := uint64(0xfffffffffffffffd) ^ num
		tem13 := tem11 ^ uint64(0x9000000000000000)
		tem14 := tem13 & tem12
		tem15 := tem11 | tem12
		k5 := (tem15 - tem14) & 0xFFFFFFFFFFFFFFFF

		k = append(k, intToHexStr(k5))
	}
	return k
}

func parseHexUint64(s string) (uint64, error) {
	var result uint64
	_, err := fmt.Sscanf(s, "%x", &result)
	return result, err
}

// makeArgusEorDataRound 单轮的运算
func makeArgusEorDataRound(p1, p2, k string) (string, string) {
	p2Val, _ := parseHexUint64(p2)
	p1Val, _ := parseHexUint64(p1)
	kVal, _ := parseHexUint64(k)

	p2_1 := ror64(p2Val, 0x38)
	p2_2 := ror64(p2Val, 0x3f)
	p2_4 := ror64(p2Val, 0x3e)
	p2_3 := p2_1 & p2_2
	tem1 := p1Val ^ p2_3
	tem2 := p2_4 ^ tem1

	newP1 := p2
	newP2 := intToHexStr(kVal ^ tem2)
	return newP1, newP2
}

// pkcs7Pad PKCS7填充
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

// MakeArgusEorData 对protobuf进行n*72轮加密运算
func MakeArgusEorData(protobuf, key string) string {
	protobufBytes, _ := hex.DecodeString(protobuf)
	protobufBytes = pkcs7Pad(protobufBytes, 16)
	protobuf = hex.EncodeToString(protobufBytes)

	res := ""
	k := []string{key[:16], key[16:32], key[32:48], key[48:64]}
	keyList := makeArgusEorDataKeyList(k)

	for i := 0; i < len(protobuf)/32; i++ {
		p1 := littleEndianToBig(protobuf[i*32 : i*32+16])
		p2 := littleEndianToBig(protobuf[i*32+16 : i*32+32])

		for j := 0; j < 72; j++ {
			p1, p2 = makeArgusEorDataRound(p1, p2, keyList[j])
		}
		res += toFixedHex(p1, 16) + toFixedHex(p2, 16)
	}
	return res
}

// MakeArgusAesData 将加密结果与固定字符串进行异或
func MakeArgusAesData(eor1, eor2, aes3, p14_1 string) string {
	res := "ec" // 与app的包名有关

	randStr := toFixedHex(makeRand(), 8)

	p14Val, _ := parseHexUint64(p14_1)
	x18Val := ((((p14Val & 0x3f) << 0x2e) | 0x1800000000000000) | 0x100000000000 | 0x100000000) >> 32
	x18 := toFixedHex(intToHexStr(x18Val), 8)

	res += littleEndianToBig(x18 + randStr)

	for i := len(eor1)/32 - 1; i >= 0; i-- {
		eor2Val, _ := parseHexUint64(eor2)
		hex1Val, _ := parseHexUint64(eor1[i*32 : i*32+8])
		hex2Val, _ := parseHexUint64(eor1[i*32+8 : i*32+16])
		hex3Val, _ := parseHexUint64(eor1[i*32+16 : i*32+24])
		hex4Val, _ := parseHexUint64(eor1[i*32+24 : i*32+32])

		hex1 := intToHexStr(hex1Val ^ eor2Val)
		hex2 := intToHexStr(hex2Val ^ eor2Val)
		hex3 := intToHexStr(hex3Val ^ eor2Val)
		hex4 := intToHexStr(hex4Val ^ eor2Val)

		res += toFixedHex(hex3, 8) + toFixedHex(hex4, 8) + toFixedHex(hex1, 8) + toFixedHex(hex2, 8)
	}

	res += eor2 + eor2
	res += aes3

	// 生成填充
	mallocAdr := uint64(rand.Int63n(0x7b0c6fffff-0x7b0c611111) + 0x7b0c611111)

	resBytes, _ := hex.DecodeString(res[:16])
	xorVal := resBytes[0] ^ resBytes[1] ^ resBytes[2] ^ resBytes[3] ^ resBytes[4] ^ resBytes[5] ^ resBytes[6] ^ resBytes[7]

	res += toFixedHex(intToHexStr((mallocAdr>>0x16)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x14)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x12)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x10)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0xe)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0xc)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0xa)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x8)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x6)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x4)&0xff), 2)
	res += toFixedHex(intToHexStr((mallocAdr>>0x2)&0xff), 2)
	res += toFixedHex(intToHexStr(uint64(xorVal)), 2)
	res += "0d"

	return res
}

// MakeArgusAes AES加密
func MakeArgusAes(data []byte, signKey string) string {
	hexKey := fromBase64(signKey)
	hexKeyBytes, _ := hex.DecodeString(hexKey)

	keyHash := md5.Sum(hexKeyBytes[:16])
	ivHash := md5.Sum(hexKeyBytes[16:32])

	key := keyHash[:]
	iv := ivHash[:]

	block, _ := aes.NewCipher(key)
	mode := cipher.NewCBCEncrypter(block, iv)

	ciphertext := make([]byte, len(data))
	mode.CryptBlocks(ciphertext, data)

	return hex.EncodeToString(ciphertext)
}

// MakeArgus 生成X-Argus签名
func MakeArgus(protobuf, p14_1 string) string {
	signKey := "wC8lD4bMTxmNVwY5jSkqi3QWmrphr/58ugLko7UZgWM="

	res1, res3, key := MakeArgusRes1Aes3AndKey(signKey)
	eor1 := MakeArgusEorData(protobuf, key)

	// 计算eor2
	res3Bytes, _ := hex.DecodeString(res3)
	tem := int(res3Bytes[0])
	tem1 := int(res3Bytes[1])
	eor2 := intToHexStr(uint64(^((((tem << 0xb) | tem1) ^ (tem >> 5)) ^ tem | 0) & 0xffffffff))

	aesData := MakeArgusAesData(eor1, eor2, res3, p14_1)
	aesDataBytes, _ := hex.DecodeString(aesData)
	res2 := MakeArgusAes(aesDataBytes, signKey)

	forBase64 := res1 + res2
	forBase64Bytes, _ := hex.DecodeString(forBase64)
	argus := toBase64(forBase64Bytes)

	return argus
}
