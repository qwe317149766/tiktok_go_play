package endecode

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
)

// MssdkEncrypt 对应 python 的 mssdk_encrypt(pb, is_report)：
// - pbHex: protobuf 的 hex 字符串
// - isReport: 是否 report 协议（当前调用处一般传 false）
// - paddedZlibTotalHexLen: 兼容仓库里的 Go 调用方式；用于固定 (4字节长度前缀 + zlib数据) 的 hex 总长度
//   例如 token 传 1274（即 zlib 1266 + 8），seed 传 170。
func MssdkEncrypt(pbHex string, isReport bool, paddedZlibTotalHexLen int) (string, error) {
	pbHex = strings.TrimSpace(pbHex)
	if pbHex == "" || len(pbHex)%2 != 0 {
		return "", fmt.Errorf("invalid pb hex")
	}
	pbBytes, err := hex.DecodeString(pbHex)
	if err != nil {
		return "", err
	}

	// zlib compress (level=1)，并按 paddedZlibTotalHexLen 做 0 填充（与 Python token_test/seed_test 一致的意图）
	zlibHex, err := zlibCompressHex(pbBytes, 1)
	if err != nil {
		return "", err
	}

	pbLen := len(pbBytes)
	lenPrefix := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenPrefix, uint32(pbLen))
	zlibWithLenHex := hex.EncodeToString(lenPrefix) + zlibHex

	if paddedZlibTotalHexLen > 0 {
		need := paddedZlibTotalHexLen - len(zlibWithLenHex)
		if need < 0 {
			// 不强行截断，保持可用（否则后续 byte_one/CRC 都会错）
			return "", fmt.Errorf("paddedZlibTotalHexLen too small: want %d, got %d", paddedZlibTotalHexLen, len(zlibWithLenHex))
		}
		if need > 0 {
			zlibWithLenHex += strings.Repeat("0", need)
		}
	}

	// byte_one / two_part（CRC变体）
	lastByte, err := hexByte(zlibWithLenHex[len(zlibWithLenHex)-2:])
	if err != nil {
		return "", err
	}
	byteOne := byte((((int(lastByte) ^ (pbLen & 0xff)) << 1) & 0xf8) | 0x07)
	partTwo := makeTwoPart(zlibWithLenHex)
	forXteaHex := fmt.Sprintf("%02x%s%s", byteOne, partTwo, zlibWithLenHex)

	// XTEA-CBC
	keyHex := getXTEAKey(isReport)
	iv4 := rand.Intn(0x10) + 0xC0133EB0 // [0xc0133eb0, 0xc0133ebf]
	iv4Hex := fmt.Sprintf("%08x", iv4)
	xteaEnc, err := cbcXTEA(iv4Hex+"27042020", keyHex, forXteaHex, true)
	if err != nil {
		return "", err
	}

	// AES 输入拼装： (firstByte^0x03) + xteaEnc + iv4
	firstXteaByte, err := hexByte(xteaEnc[:2])
	if err != nil {
		return "", err
	}
	modified := firstXteaByte ^ 0x03
	forAES := fmt.Sprintf("%02x%s%s", modified, xteaEnc, iv4Hex)

	// 最外层 AES-CBC + PKCS7
	out, err := aesEncryptHex(forAES)
	if err != nil {
		return "", err
	}
	return out, nil
}

// MssdkDecrypt 对应 python 的 mssdk_decrypt(encrypted_hex, is_report, is_request)。
func MssdkDecrypt(encryptedHex string, isReport bool, isRequest bool) (string, error) {
	encryptedHex = strings.TrimSpace(encryptedHex)
	if encryptedHex == "" || len(encryptedHex)%2 != 0 {
		return "", fmt.Errorf("invalid encrypted hex")
	}

	aesPlainHex, err := aesDecryptHex(encryptedHex)
	if err != nil {
		return "", err
	}
	aesPlainBytes, err := hex.DecodeString(aesPlainHex)
	if err != nil {
		return "", err
	}
	if len(aesPlainBytes) < 1+4 {
		return "", errors.New("aes plaintext too short")
	}

	// python: xtea_encrypted_hex = decrypted_aes_bytes[1:len-4]
	xteaEncHex := hex.EncodeToString(aesPlainBytes[1 : len(aesPlainBytes)-4])
	iv4Hex := hex.EncodeToString(aesPlainBytes[len(aesPlainBytes)-4:])

	keyHex := getXTEAKey(isReport)
	xteaPlainHex, err := cbcXTEA(iv4Hex+"27042020", keyHex, xteaEncHex, false)
	if err != nil {
		return "", err
	}

	// python: 根据 is_request 拼回 zlib header，然后解压
	var header string
	if isRequest {
		header = "7801"
	} else {
		header = "78da"
	}

	idx := strings.Index(xteaPlainHex, header)
	if idx < 0 {
		return "", fmt.Errorf("zlib header not found")
	}
	zlibHex := header + xteaPlainHex[idx+len(header):]

	zlibBytes, err := hex.DecodeString(zlibHex)
	if err != nil {
		return "", err
	}
	plain, err := zlibDecompress(zlibBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(plain), nil
}

func zlibCompressHex(data []byte, level int) (string, error) {
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, level)
	if err != nil {
		return "", err
	}
	if _, err := zw.Write(data); err != nil {
		zw.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

func zlibDecompress(data []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func hexByte(twoHex string) (byte, error) {
	b, err := hex.DecodeString(twoHex)
	if err != nil || len(b) != 1 {
		return 0, fmt.Errorf("invalid hex byte: %q", twoHex)
	}
	return b[0], nil
}

func getXTEAKey(isReport bool) string {
	if !isReport {
		return "782399bdfacedead3230313030343034"
	}
	// report key = get_tea_report_key() + "3230313030343034"
	return getTeaReportKey() + "3230313030343034"
}

// getTeaReportKey 直接移植 python get_tea_report_key() 的位运算逻辑。
func getTeaReportKey() string {
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
	w14 = 0x216249
	w16 = 0x3f47825
	w10 = w10 ^ w14

	w14 = w11 | w16
	w11 = w11 & 0x1000000
	w10 = w13 | w10
	w11 = w11 | 0x200000
	w10 = (w10 - w17) & 0xFFFFFFFF

	first := make([]byte, 4)
	binary.LittleEndian.PutUint32(first, w10)

	w11 = (w14 - w11) & 0xFFFFFFFF
	w8 := w15 &^ w11
	w10 = w11 &^ w15
	w12 = (w12 + w2) & 0xFFFFFFFF
	w8 = w8 | w10
	w10 = w8 | w12
	w8 = w8 & w12
	w8 = (w10 - w8) & 0xFFFFFFFF

	second := make([]byte, 4)
	binary.LittleEndian.PutUint32(second, w8)
	return hex.EncodeToString(append(first, second...))
}

// --- XTEA + CBC ---

func cbcXTEA(ivHex, keyHex, dataHex string, encrypt bool) (string, error) {
	iv, err := hex.DecodeString(ivHex)
	if err != nil || len(iv) != 8 {
		return "", fmt.Errorf("invalid iv")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 16 {
		return "", fmt.Errorf("invalid key")
	}
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return "", err
	}

	// python: padding_len = 16 - (len(data) % 8); if padding_len != 16: append zero bytes
	if rem := len(data) % 8; rem != 0 {
		paddingLen := 16 - rem
		data = append(data, make([]byte, paddingLen)...)
	}

	rounds := xteaRoundsFromIV(iv[:4])
	x := newXTEA(key, rounds)

	chaining := make([]byte, 8)
	copy(chaining, iv)
	out := make([]byte, 0, len(data))

	for i := 0; i < len(data); i += 8 {
		block := data[i : i+8]
		if encrypt {
			tmp := xor8(block, chaining)
			enc := x.encryptBlock(tmp)
			out = append(out, enc...)
			copy(chaining, enc)
		} else {
			dec := x.decryptBlock(block)
			plain := xor8(dec, chaining)
			out = append(out, plain...)
			copy(chaining, block)
		}
	}
	return hex.EncodeToString(out), nil
}

func xteaRoundsFromIV(ivFirst4 []byte) int {
	v14 := binary.LittleEndian.Uint32(ivFirst4)
	m := v14 % 5
	rounds := (8*(((2*m)&8)|m) ^ 0x20) & 0xFFFFFFFF
	return int(rounds)
}

func xor8(a, b []byte) []byte {
	out := make([]byte, 8)
	for i := 0; i < 8; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

type xteaCipher struct {
	rounds uint32
	delta  uint32
	k      [4]uint32
}

func newXTEA(key16 []byte, rounds int) *xteaCipher {
	var k [4]uint32
	// python: struct.unpack('>4I', key)
	for i := 0; i < 4; i++ {
		k[i] = binary.BigEndian.Uint32(key16[i*4 : i*4+4])
	}
	return &xteaCipher{
		rounds: uint32(rounds),
		delta:  0x9E3779B9,
		k:      k,
	}
}

func (x *xteaCipher) encryptBlock(block8 []byte) []byte {
	v0 := binary.BigEndian.Uint32(block8[0:4])
	v1 := binary.BigEndian.Uint32(block8[4:8])
	var s uint32 = 0
	for i := uint32(0); i < x.rounds; i++ {
		v0 = v0 + ((((v1 << 4) ^ (v1 >> 5)) + v1) ^ (s + x.k[s&3]))
		s = s + x.delta
		v1 = v1 + ((((v0 << 4) ^ (v0 >> 5)) + v0) ^ (s + x.k[(s>>11)&3]))
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], v0)
	binary.BigEndian.PutUint32(out[4:8], v1)
	return out
}

func (x *xteaCipher) decryptBlock(block8 []byte) []byte {
	v0 := binary.BigEndian.Uint32(block8[0:4])
	v1 := binary.BigEndian.Uint32(block8[4:8])
	s := x.delta * x.rounds
	for i := uint32(0); i < x.rounds; i++ {
		v1 = v1 - ((((v0 << 4) ^ (v0 >> 5)) + v0) ^ (s + x.k[(s>>11)&3]))
		s = s - x.delta
		v0 = v0 - ((((v1 << 4) ^ (v1 >> 5)) + v1) ^ (s + x.k[s&3]))
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], v0)
	binary.BigEndian.PutUint32(out[4:8], v1)
	return out
}

// --- AES layer ---

var (
	aesKey = mustHex16("b8d72ddec05142948bbf2dc81d63759c")
	aesIV  = mustHex16("d6c3969582f9ac5313d39c180b54a2bc")
)

func mustHex16(h string) []byte {
	b, _ := hex.DecodeString(h)
	return b
}

func pkcs7Pad(in []byte, blockSize int) []byte {
	padLen := blockSize - (len(in) % blockSize)
	if padLen == 0 {
		padLen = blockSize
	}
	return append(in, bytes.Repeat([]byte{byte(padLen)}, padLen)...)
}

func aesEncryptHex(plainHex string) (string, error) {
	plain, err := hex.DecodeString(plainHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	plain = pkcs7Pad(plain, block.BlockSize())
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, aesIV).CryptBlocks(out, plain)
	return hex.EncodeToString(out), nil
}

// aesDecryptHex 对应 python last_aes_decrypt：用最后一个字节作为 pad length 直接截断（不校验）。
func aesDecryptHex(cipherHex string) (string, error) {
	ct, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", err
	}
	if len(ct)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext not multiple of block size")
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, aesIV).CryptBlocks(plain, ct)
	if len(plain) == 0 {
		return "", errors.New("empty plaintext")
	}
	padLen := int(plain[len(plain)-1])
	if padLen <= 0 || padLen > len(plain) {
		return "", errors.New("invalid padding length")
	}
	return hex.EncodeToString(plain[:len(plain)-padLen]), nil
}

// --- CRC-ish helper (make_two_part) ---

var crcTable = [...]uint16{
	0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50A5, 0x60C6, 0x70E7,
	0x8108, 0x9129, 0xA14A, 0xB16B, 0xC18C, 0xD1AD, 0xE1CE, 0xF1EF,
	0x1231, 0x0210, 0x3273, 0x2252, 0x52B5, 0x4294, 0x72F7, 0x62D6,
	0x9339, 0x8318, 0xB37B, 0xA35A, 0xD3BD, 0xC39C, 0xF3FF, 0xE3DE,
	0x2462, 0x3443, 0x0420, 0x1401, 0x64E6, 0x74C7, 0x44A4, 0x5485,
	0xA56A, 0xB54B, 0x8528, 0x9509, 0xE5EE, 0xF5CF, 0xC5AC, 0xD58D,
	0x3653, 0x2672, 0x1611, 0x0630, 0x76D7, 0x66F6, 0x5695, 0x46B4,
	0xB75B, 0xA77A, 0x9719, 0x8738, 0xF7DF, 0xE7FE, 0xD79D, 0xC7BC,
	0x48C4, 0x58E5, 0x6886, 0x78A7, 0x0840, 0x1861, 0x2802, 0x3823,
	0xC9CC, 0xD9ED, 0xE98E, 0xF9AF, 0x8948, 0x9969, 0xA90A, 0xB92B,
	0x5AF5, 0x4AD4, 0x7AB7, 0x6A96, 0x1A71, 0x0A50, 0x3A33, 0x2A12,
	0xDBFD, 0xCBDC, 0xFBBF, 0xEB9E, 0x9B79, 0x8B58, 0xBB3B, 0xAB1A,
	0x6CA6, 0x7C87, 0x4CE4, 0x5CC5, 0x2C22, 0x3C03, 0x0C60, 0x1C41,
	0xEDAE, 0xFD8F, 0xCDEC, 0xDDCD, 0xAD2A, 0xBD0B, 0x8D68, 0x9D49,
	0x7E97, 0x6EB6, 0x5ED5, 0x4EF4, 0x3E13, 0x2E32, 0x1E51, 0x0E70,
	0xFF9F, 0xEFBE, 0xDFDD, 0xCFFC, 0xBF1B, 0xAF3A, 0x9F59, 0x8F78,
	0x9188, 0x81A9, 0xB1CA, 0xA1EB, 0xD10C, 0xC12D, 0xF14E, 0xE16F,
	0x1080, 0x00A1, 0x30C2, 0x20E3, 0x5004, 0x4025, 0x7046, 0x6067,
	0x83B9, 0x9398, 0xA3FB, 0xB3DA, 0xC33D, 0xD31C, 0xE37F, 0xF35E,
	0x02B1, 0x1290, 0x22F3, 0x32D2, 0x4235, 0x5214, 0x6277, 0x7256,
	0xB5EA, 0xA5CB, 0x95A8, 0x8589, 0xF56E, 0xE54F, 0xD52C, 0xC50D,
	0x34E2, 0x24C3, 0x14A0, 0x0481, 0x7466, 0x6447, 0x5424, 0x4405,
	0xA7DB, 0xB7FA, 0x8799, 0x97B8, 0xE75F, 0xF77E, 0xC71D, 0xD73C,
	0x26D3, 0x36F2, 0x0691, 0x16B0, 0x6657, 0x7676, 0x4615, 0x5634,
	0xD94C, 0xC96D, 0xF90E, 0xE92F, 0x99C8, 0x89E9, 0xB98A, 0xA9AB,
	0x5844, 0x4865, 0x7806, 0x6827, 0x18C0, 0x08E1, 0x3882, 0x28A3,
	0xCB7D, 0xDB5C, 0xEB3F, 0xFB1E, 0x8BF9, 0x9BD8, 0xABBB, 0xBB9A,
	0x4A75, 0x5A54, 0x6A37, 0x7A16, 0x0AF1, 0x1AD0, 0x2AB3, 0x3A92,
	0xFD2E, 0xED0F, 0xDD6C, 0xCD4D, 0xBDAA, 0xAD8B, 0x9DE8, 0x8DC9,
	0x7C26, 0x6C07, 0x5C64, 0x4C45, 0x3CA2, 0x2C83, 0x1CE0, 0x0CC1,
	0xEF1F, 0xFF3E, 0xCF5D, 0xDF7C, 0xAF9B, 0xBFBA, 0x8FD9, 0x9FF8,
	0x6E17, 0x7E36, 0x4E55, 0x5E74, 0x2E93, 0x3EB2, 0x0ED1, 0x1EF0,
}

func makeTwoPart(hexStr string) string {
	data, err := hex.DecodeString(hexStr)
	if err != nil || len(data) < 1 {
		return ""
	}

	var hashVal uint32 = 0
	for _, b := range data {
		byte1 := (hashVal >> 8) & 0xFF
		idx := uint8(b) ^ uint8(byte1)
		lookup := uint32(crcTable[idx])
		hashVal = (lookup ^ (hashVal << 8)) & 0xFFFFFFFF
	}

	dataLen := len(data)
	w9 := (-dataLen) & 0x7
	w10 := w9 ^ 7
	w9 = (w9 << 1) & 0b111
	w24 := (w9 + w10) & 0xFFFFFFFF
	w9i := w24 + 3
	_ = w9i // python 的分支在此处等价于 w24（w24 总是非负）
	w9i = w24 &^ 3
	w21 := w24 - w9i

	shift := (4 - w21) * 8
	mask := uint32(0xFFFFFFFF)
	truncated := ((hashVal << shift) & mask) >> shift

	out := make([]byte, w21)
	for i := 0; i < w21; i++ {
		out[w21-1-i] = byte(truncated & 0xFF)
		truncated >>= 8
	}
	return hex.EncodeToString(out)
}


