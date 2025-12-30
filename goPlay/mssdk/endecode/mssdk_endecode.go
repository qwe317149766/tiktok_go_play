package endecode

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// TT_XTEA 结构 - 完全匹配Python实现
type TT_XTEA struct {
	rounds uint32
	delta  uint32
	key    [4]uint32
}

// NewTT_XTEA 创建新的XTEA实例
func NewTT_XTEA(key []byte, rounds uint32) (*TT_XTEA, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("密钥长度必须是 16 字节")
	}
	var k [4]uint32
	for i := 0; i < 4; i++ {
		k[i] = binary.BigEndian.Uint32(key[i*4 : (i+1)*4])
	}
	return &TT_XTEA{
		rounds: rounds,
		delta:  0x9E3779B9,
		key:    k,
	}, nil
}

// EncryptBlock 加密8字节块
func (xtea *TT_XTEA) EncryptBlock(block []byte) ([]byte, error) {
	if len(block) != 8 {
		return nil, fmt.Errorf("数据块长度必须是 8 字节")
	}
	v0 := binary.BigEndian.Uint32(block[:4])
	v1 := binary.BigEndian.Uint32(block[4:])
	var s uint32 = 0
	for i := uint32(0); i < xtea.rounds; i++ {
		v0 = (v0 + ((((v1 << 4) ^ (v1 >> 5)) + v1) ^ (s + xtea.key[s&3]))) & 0xFFFFFFFF
		s = (s + xtea.delta) & 0xFFFFFFFF
		v1 = (v1 + ((((v0 << 4) ^ (v0 >> 5)) + v0) ^ (s + xtea.key[(s>>11)&3]))) & 0xFFFFFFFF
	}
	result := make([]byte, 8)
	binary.BigEndian.PutUint32(result[:4], v0)
	binary.BigEndian.PutUint32(result[4:], v1)
	return result, nil
}

// DecryptBlock 解密8字节块
func (xtea *TT_XTEA) DecryptBlock(block []byte) ([]byte, error) {
	if len(block) != 8 {
		return nil, fmt.Errorf("数据块长度必须是 8 字节")
	}
	v0 := binary.BigEndian.Uint32(block[:4])
	v1 := binary.BigEndian.Uint32(block[4:])
	s := (xtea.delta * xtea.rounds) & 0xFFFFFFFF
	for i := uint32(0); i < xtea.rounds; i++ {
		v1 = (v1 - ((((v0 << 4) ^ (v0 >> 5)) + v0) ^ (s + xtea.key[(s>>11)&3]))) & 0xFFFFFFFF
		s = (s - xtea.delta) & 0xFFFFFFFF
		v0 = (v0 - ((((v1 << 4) ^ (v1 >> 5)) + v1) ^ (s + xtea.key[s&3]))) & 0xFFFFFFFF
	}
	result := make([]byte, 8)
	binary.BigEndian.PutUint32(result[:4], v0)
	binary.BigEndian.PutUint32(result[4:], v1)
	return result, nil
}

// ZlibCompress 压缩数据 (level=1)
func ZlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		return nil, err
	}
	w.Close()
	return buf.Bytes(), nil
}

// ZlibCompressWithFixedLength 压缩数据并填充到固定长度 - 完全匹配Python实现
// fixedLengthHexChars: 固定长度（hex字符串的字符数，不是字节数）
func ZlibCompressWithFixedLength(data []byte, fixedLengthHexChars int) (string, error) {
	compressed, err := ZlibCompress(data)
	if err != nil {
		return "", err
	}
	resHex := hex.EncodeToString(compressed)
	// Python: return res+(170-len(res))*"0"
	// fmt.Println("resHexlength===>", len(resHex))
	// if len(resHex) < fixedLengthHexChars {
	// 	resHex += strings.Repeat("0", fixedLengthHexChars-len(resHex))
	// }
	resHex1 := ""
	if len(resHex) < fixedLengthHexChars {
		resHex1 = resHex + strings.Repeat("0", fixedLengthHexChars-len(resHex))
	} else {
		resHex1 = resHex
	}
	if len(resHex1)%8 != 2 {
		resHex1 += strings.Repeat("0", 10-len(resHex1)%8)
	}
	if len(resHex1) > len(resHex) {
		// fmt.Println("resHexlength1===>", len(resHex1))
		return resHex1, nil
	} else {
		return resHex, nil
	}
}

// ZlibDecompress 解压数据
func ZlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// MakeRand 生成4字节16进制随机数
func MakeRand() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Word19DED0 查找表
var Word19DED0 = []uint32{
	0x0, 0x1021, 0x2042, 0x3063, 0x4084, 0x50A5, 0x60C6, 0x70E7, 0x8108, 0x9129, 0xA14A, 0xB16B, 0xC18C, 0xD1AD, 0xE1CE, 0xF1EF,
	0x1231, 0x210, 0x3273, 0x2252, 0x52B5, 0x4294, 0x72F7, 0x62D6, 0x9339, 0x8318, 0xB37B, 0xA35A, 0xD3BD, 0xC39C, 0xF3FF, 0xE3DE,
	0x2462, 0x3443, 0x420, 0x1401, 0x64E6, 0x74C7, 0x44A4, 0x5485, 0xA56A, 0xB54B, 0x8528, 0x9509, 0xE5EE, 0xF5CF, 0xC5AC, 0xD58D,
	0x3653, 0x2672, 0x1611, 0x630, 0x76D7, 0x66F6, 0x5695, 0x46B4, 0xB75B, 0xA77A, 0x9719, 0x8738, 0xF7DF, 0xE7FE, 0xD79D, 0xC7BC,
	0x48C4, 0x58E5, 0x6886, 0x78A7, 0x840, 0x1861, 0x2802, 0x3823, 0xC9CC, 0xD9ED, 0xE98E, 0xF9AF, 0x8948, 0x9969, 0xA90A, 0xB92B,
	0x5AF5, 0x4AD4, 0x7AB7, 0x6A96, 0x1A71, 0xA50, 0x3A33, 0x2A12, 0xDBFD, 0xCBDC, 0xFBBF, 0xEB9E, 0x9B79, 0x8B58, 0xBB3B, 0xAB1A,
	0x6CA6, 0x7C87, 0x4CE4, 0x5CC5, 0x2C22, 0x3C03, 0xC60, 0x1C41, 0xEDAE, 0xFD8F, 0xCDEC, 0xDDCD, 0xAD2A, 0xBD0B, 0x8D68, 0x9D49,
	0x7E97, 0x6EB6, 0x5ED5, 0x4EF4, 0x3E13, 0x2E32, 0x1E51, 0xE70, 0xFF9F, 0xEFBE, 0xDFDD, 0xCFFC, 0xBF1B, 0xAF3A, 0x9F59, 0x8F78,
	0x9188, 0x81A9, 0xB1CA, 0xA1EB, 0xD10C, 0xC12D, 0xF14E, 0xE16F, 0x1080, 0xA1, 0x30C2, 0x20E3, 0x5004, 0x4025, 0x7046, 0x6067,
	0x83B9, 0x9398, 0xA3FB, 0xB3DA, 0xC33D, 0xD31C, 0xE37F, 0xF35E, 0x2B1, 0x1290, 0x22F3, 0x32D2, 0x4235, 0x5214, 0x6277, 0x7256,
	0xB5EA, 0xA5CB, 0x95A8, 0x8589, 0xF56E, 0xE54F, 0xD52C, 0xC50D, 0x34E2, 0x24C3, 0x14A0, 0x481, 0x7466, 0x6447, 0x5424, 0x4405,
	0xA7DB, 0xB7FA, 0x8799, 0x97B8, 0xE75F, 0xF77E, 0xC71D, 0xD73C, 0x26D3, 0x36F2, 0x691, 0x16B0, 0x6657, 0x7676, 0x4615, 0x5634,
	0xD94C, 0xC96D, 0xF90E, 0xE92F, 0x99C8, 0x89E9, 0xB98A, 0xA9AB, 0x5844, 0x4865, 0x7806, 0x6827, 0x18C0, 0x8E1, 0x3882, 0x28A3,
	0xCB7D, 0xDB5C, 0xEB3F, 0xFB1E, 0x8BF9, 0x9BD8, 0xABBB, 0xBB9A, 0x4A75, 0x5A54, 0x6A37, 0x7A16, 0xAF1, 0x1AD0, 0x2AB3, 0x3A92,
	0xFD2E, 0xED0F, 0xDD6C, 0xCD4D, 0xBDAA, 0xAD8B, 0x9DE8, 0x8DC9, 0x7C26, 0x6C07, 0x5C64, 0x4C45, 0x3CA2, 0x2C83, 0x1CE0, 0xCC1,
	0xEF1F, 0xFF3E, 0xCF5D, 0xDF7C, 0xAF9B, 0xBFBA, 0x8FD9, 0x9FF8, 0x6E17, 0x7E36, 0x4E55, 0x5E74, 0x2E93, 0x3EB2, 0xED1, 0x1EF0,
}

// MakeTwoPart 计算two_part - 完全匹配Python实现
func MakeTwoPart(dataHex string) (string, error) {
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return "", err
	}
	if len(data) < 1 {
		return "0", nil
	}

	var hashVal uint32 = 0
	for _, currentByte := range data {
		byte1OfHash := (hashVal >> 8) & 0xFF
		tableIndex := uint32(currentByte) ^ byte1OfHash
		lookupValue := Word19DED0[tableIndex]
		shiftedHash := hashVal << 8
		newHash := lookupValue ^ shiftedHash
		hashVal = newHash & 0xFFFFFFFF
	}

	// 计算长度 - 完全匹配Python实现
	dataLen := len(data)
	w9 := uint32((-dataLen) & 0x7)
	w10 := w9 ^ 7
	w9 = (w9 << 1) & 0b111
	w24 := (w9 + w10) & 0xFFFFFFFF
	// Python: w9 = (w24 + 3) & 0xFFFFFFFF
	//        w9 = w9 if w24 < 0 else w24
	// 在Python中，w24是32位无符号整数，但Python的整数比较会将其视为有符号整数
	// 如果w24 >= 0x80000000，在Python中会被视为负数
	// 但实际计算中w24总是小值，所以w24 < 0永远不会为真，因此总是使用w24
	// 为了完全匹配Python代码，我们仍然计算w9Temp，但总是使用w24
	w9Temp := (w24 + 3) & 0xFFFFFFFF
	// 检查w24是否会被Python视为负数（最高位为1）
	if w24 >= 0x80000000 {
		w9 = w9Temp
	} else {
		w9 = w24
	}
	w9 = w9 & 0xFFFFFFFC
	w21 := w24 - w9
	shift := (4 - w21) * 8

	// 转换为字节
	val := (hashVal << shift) & 0xFFFFFFFF
	val = val >> shift

	fullBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(fullBytes, val)
	resultBytes := fullBytes[4-w21:]

	return hex.EncodeToString(resultBytes), nil
}

// GetTeaReportKey 获取report协议的XTEA密钥
func GetTeaReportKey() string {
	data := []byte("v05.02.00-ov-android")
	w12 := uint32(0x26000)
	w11 := uint32(0x280000)
	w13 := uint32(0x9000)
	w17 := (w12 + (3 << 12)) & 0xFFFFFFFF
	w14 := uint32(0x15000000)
	w15 := uint32(data[2])
	w0 := uint32(data[5])
	w10 := uint32(data[8])
	w16 := uint32(0x5f00000)
	w2 := (w15 << 8) & 0xFFFFFFFF
	w17 = w2 ^ w17
	w13 = w2 & w13
	w2 = w12 | 0x200
	w11 = (w11 | (w0 << 20)) & 0xFFFFFFFF
	w0 = (w0 << 0x18) & 0xFFFFFFFF
	w13 = w17 | w13
	w17 = w17 & w2
	w2 = w0 & 0xfdffffff
	w0 = w0 & w14
	w14 = w2 ^ w14
	w2 = (w10 << 8) & 0xFFFFFFFF
	w15 = (w15 << 0x10) & 0xFFFFFFFF
	w10 = (w10 << 0x14) & 0xFFFFFFFF
	w16 = w15 ^ w16
	w15 = w15 & 0xf00000
	w10 = w10 & 0xfeffffff
	w15 = w15 | w16
	w16 = w2 & 0x6000
	w10 = w10 | 0x38000000
	w12 = w16 ^ w12
	w10 = w11 ^ w10
	w11 = w14 | w0
	w14 = uint32(0x216249)
	w16 = uint32(0x3f47825)
	w10 = w10 ^ w14

	w14 = w11 | w16
	w11 = w11 & 0x1000000
	w10 = w13 | w10
	w11 = w11 | 0x200000
	w10 = (w10 - w17) & 0xFFFFFFFF
	dataFirst4Bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(dataFirst4Bytes, w10)

	w11 = (w14 - w11) & 0xFFFFFFFF
	w8 := w15 & (^w11)
	w10 = w11 & (^w15)
	w12 = (w12 + w2) & 0xFFFFFFFF
	w8 = w8 | w10
	w10 = w8 | w12
	w8 = w8 & w12
	w8 = (w10 - w8) & 0xFFFFFFFF
	dataSecond4Bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(dataSecond4Bytes, w8)

	return hex.EncodeToString(append(dataFirst4Bytes, dataSecond4Bytes...))
}

// GetXTEAKey 获取XTEA密钥
func GetXTEAKey(isReport bool) string {
	if !isReport {
		return "782399bdfacedead3230313030343034"
	}
	return GetTeaReportKey() + "3230313030343034"
}

// xorBytes 异或两个字节数组
func xorBytes(b1, b2 []byte) []byte {
	result := make([]byte, len(b1))
	for i := range b1 {
		if i < len(b2) {
			result[i] = b1[i] ^ b2[i]
		} else {
			result[i] = b1[i]
		}
	}
	return result
}

// IntToHexStr 整数转16进制字符串
func IntToHexStr(num int) string {
	return fmt.Sprintf("%02x", num)
}

// CBCXTEAEncryptOrDecrypt CBC模式XTEA加解密 - 完全匹配Python实现
func CBCXTEAEncryptOrDecrypt(ivHex, keyHex, dataHex string, isEncrypt bool) (string, error) {
	dataBytes, err := hex.DecodeString(dataHex)
	if err != nil {
		return "", err
	}

	// 填充到8字节对齐 - 完全匹配Python实现
	// Python: padding_len = 16 - (len(data_bytes) % 8)
	//         if padding_len != 16:
	//             data_bytes += bytearray(padding_len)
	// 这个逻辑的含义：
	// - 当 len % 8 == 0 时，padding_len = 16，不填充（因为 padding_len == 16）
	// - 当 len % 8 == 1 时，padding_len = 15，填充15字节
	// - 当 len % 8 == 2 时，padding_len = 14，填充14字节
	// - ...
	// - 当 len % 8 == 7 时，padding_len = 9，填充9字节
	paddingLen := 16 - (len(dataBytes) % 8)
	if paddingLen != 16 {
		dataBytes = append(dataBytes, make([]byte, paddingLen)...)
	}

	// 计算轮数
	ivBytes, err := hex.DecodeString(ivHex)
	if err != nil {
		return "", err
	}
	v14 := binary.LittleEndian.Uint32(ivBytes[:4])
	rounds := uint32((8 * (((2 * (v14 % 5)) & 8) | (v14 % 5))) ^ 0x20)

	derivedKey, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", err
	}

	cipher, err := NewTT_XTEA(derivedKey, rounds)
	if err != nil {
		return "", err
	}

	chainingBlock := ivBytes
	outputData := []byte{}

	for i := 0; i < len(dataBytes); i += 8 {
		currentBlock := dataBytes[i : i+8]

		if isEncrypt {
			blockToEncrypt := xorBytes(currentBlock, chainingBlock)
			encryptedBlock, err := cipher.EncryptBlock(blockToEncrypt)
			if err != nil {
				return "", err
			}
			outputData = append(outputData, encryptedBlock...)
			chainingBlock = encryptedBlock
		} else {
			decryptedBlock, err := cipher.DecryptBlock(currentBlock)
			if err != nil {
				return "", err
			}
			plaintextBlock := xorBytes(decryptedBlock, chainingBlock)
			outputData = append(outputData, plaintextBlock...)
			chainingBlock = currentBlock
		}
	}
	return hex.EncodeToString(outputData), nil
}

// LastAESEncrypt 最后一层AES加密 - 完全匹配Python实现
func LastAESEncrypt(dataHex string) (string, error) {
	dataBytes, err := hex.DecodeString(dataHex)
	if err != nil {
		return "", err
	}
	key, _ := hex.DecodeString("b8d72ddec05142948bbf2dc81d63759c")
	iv, _ := hex.DecodeString("d6c3969582f9ac5313d39c180b54a2bc")

	// PKCS7 填充
	padding := aes.BlockSize - (len(dataBytes) % aes.BlockSize)
	paddedData := append(dataBytes, bytes.Repeat([]byte{byte(padding)}, padding)...)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	ciphertext := make([]byte, len(paddedData))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, paddedData)

	return hex.EncodeToString(ciphertext), nil
}

// LastAESDecrypt 最后一层AES解密
func LastAESDecrypt(ciphertextHex string) (string, error) {
	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", err
	}
	key, _ := hex.DecodeString("b8d72ddec05142948bbf2dc81d63759c")
	iv, _ := hex.DecodeString("d6c3969582f9ac5313d39c180b54a2bc")

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	decryptedPadded := make([]byte, len(ciphertext))
	mode.CryptBlocks(decryptedPadded, ciphertext)

	// 去除填充
	padLength := int(decryptedPadded[len(decryptedPadded)-1])
	if padLength > len(decryptedPadded) {
		return "", fmt.Errorf("无效的填充长度")
	}
	return hex.EncodeToString(decryptedPadded[:len(decryptedPadded)-padLength]), nil
}

// MssdkEncrypt MSSDK加密 - 完全匹配Python实现
// 参数: pbHex是hex字符串, isReport是协议类型, fixedZlibLength是zlib压缩后的固定长度（hex字符数，0表示不固定）
// 返回: 加密后的hex字符串
func MssdkEncrypt(pbHex string, isReport bool, fixedZlibLength int) (string, error) {
	pb, err := hex.DecodeString(pbHex)
	if err != nil {
		return "", err
	}

	// 1. Zlib压缩
	var zlibResHex string
	if fixedZlibLength > 0 {
		// 使用固定长度压缩
		zlibResHex, err = ZlibCompressWithFixedLength(pb, fixedZlibLength)
		if err != nil {
			return "", fmt.Errorf("zlib compress error: %v", err)
		}
	} else {
		// 不固定长度
		zlibRes, err := ZlibCompress(pb)
		if err != nil {
			return "", fmt.Errorf("zlib compress error: %v", err)
		}
		zlibResHex = hex.EncodeToString(zlibRes)
	}
	// fmt.Println("zlibResHex", zlibResHex)
	// zlibResHex = "780105c1310a80300c05d05174d4cd493c40f94d9326394eb0165c141c3cbfef4d9b28bb502e86e8ac71046a14e2d6733fcd28e64585a81ac8bd102a67a8af43dced7daeb68f1f248112f003a49711e40000000000"
	// 2. 添加长度前缀
	pbLength := len(pb)
	threePart := make([]byte, 4)
	binary.LittleEndian.PutUint32(threePart, uint32(pbLength))
	zlibResHex = hex.EncodeToString(threePart) + zlibResHex
	// 3. 计算byte_one
	lastByte, _ := hex.DecodeString(zlibResHex[len(zlibResHex)-2:])
	byteOne := IntToHexStr(int((((int(lastByte[0])^(pbLength&0xff))<<1)&0xf8)|0x7) & 0xff)

	// 4. 计算two_part
	partTwo, err := MakeTwoPart(zlibResHex)
	if err != nil {
		return "", err
	}

	// 5. 组装for_xtea
	forXtea := byteOne + partTwo + zlibResHex

	// 6. XTEA加密
	key := GetXTEAKey(isReport)
	// Python: random.randint(0xc0133eb0, 0xc0133ebf) 范围是 [0xc0133eb0, 0xc0133ebf]
	ivForByte := fmt.Sprintf("%08x", rand.Intn(16)+0xc0133eb0)
	xteaEncrypted, err := CBCXTEAEncryptOrDecrypt(ivForByte+"27042020", key, forXtea, true)
	if err != nil {
		return "", err
	}

	// 7. 处理第一个字节
	firstXteaByte, _ := hex.DecodeString(xteaEncrypted[:2])
	modifiedByte := IntToHexStr(int(firstXteaByte[0]) ^ 0x3)

	// 8. 组装for_aes
	forAes := modifiedByte + xteaEncrypted + ivForByte

	// 9. AES加密
	res, err := LastAESEncrypt(forAes)
	if err != nil {
		return "", err
	}
	return res, nil
}

// MssdkDecrypt MSSDK解密 - 完全匹配Python实现
func MssdkDecrypt(encryptedHex string, isReport bool, isRequest bool) (string, error) {
	// 1. AES解密
	decryptedAesHex, err := LastAESDecrypt(encryptedHex)
	if err != nil {
		return "", err
	}

	decryptedAesBytes, _ := hex.DecodeString(decryptedAesHex)

	// 2. 提取XTEA密文和IV
	xteaEncryptedHex := hex.EncodeToString(decryptedAesBytes[1 : len(decryptedAesBytes)-4])
	randomIvFourByte := hex.EncodeToString(decryptedAesBytes[len(decryptedAesBytes)-4:])

	// 3. XTEA解密
	key := GetXTEAKey(isReport)
	decryptedXteaHex, err := CBCXTEAEncryptOrDecrypt(randomIvFourByte+"27042020", key, xteaEncryptedHex, false)
	if err != nil {
		return "", err
	}

	// 4. 提取zlib数据
	var zlibResHex string
	if isRequest {
		parts := strings.Split(decryptedXteaHex, "7801")
		if len(parts) > 1 {
			zlibResHex = "7801" + strings.Join(parts[1:], "7801")
		}
	} else {
		parts := strings.Split(decryptedXteaHex, "78da")
		if len(parts) > 1 {
			zlibResHex = "78da" + strings.Join(parts[1:], "78da")
		}
	}

	// 5. Zlib解压
	zlibData, _ := hex.DecodeString(zlibResHex)
	originalPb, err := ZlibDecompress(zlibData)
	if err != nil {
		return "", fmt.Errorf("zlib decompress error: %v", err)
	}

	return hex.EncodeToString(originalPb), nil
}

// func main() {
// 	pb := "0a2035373439353231333830616634376163613036613332346466316665383832611213373532323638303239393332303634313037391a07616e64726f696422097630352e30322e3030"
// 	isReport := false
// 	fixedZlibLength := 170
// 	res, err := MssdkEncrypt(pb, isReport, fixedZlibLength)
// 	if err != nil {
// 		fmt.Println("encrypt error:", err)
// 		return
// 	}
// 	fmt.Println("encrypt result:", res)
// }
