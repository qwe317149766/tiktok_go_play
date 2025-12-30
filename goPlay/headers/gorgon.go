package headers

import (
	"encoding/hex"
	"fmt"
)

// makeGorgonRc4Init 初始化Gorgon RC4 Sbox
func makeGorgonRc4Init(key string) []byte {
	Sbox := make([]byte, 256)
	for i := 0; i < 256; i++ {
		Sbox[i] = byte(i)
	}

	keyBytes, _ := hex.DecodeString(key)
	prevIndex := 0

	getKeyByte := func(v87 []byte, idx int) byte {
		if idx >= 0 && idx < len(v87) {
			return v87[idx]
		}
		return 0
	}

	for i := 0; i < 256; i++ {
		j := i + 7
		if i >= 0 {
			j = i
		}
		keyOffset := i - (j & 0xFFFFFFF8)
		k := int(getKeyByte(keyBytes, keyOffset))
		b := int(Sbox[i])

		inner := 2*(prevIndex|b) - (prevIndex ^ b)
		v47 := 2*(inner|k) - (inner ^ k)

		temp2 := v47
		if v47 < 0 {
			temp2 = v47 + 255
		}
		jIdx := v47 - (temp2 & 0xFFFFFF00)
		jIdx = jIdx % 256
		if jIdx < 0 {
			jIdx += 256
		}

		Sbox[i] = Sbox[jIdx]
		prevIndex = jIdx
	}

	return Sbox
}

// makeGorgonRc4 执行Gorgon RC4加密
func makeGorgonRc4(data []byte, keyLen int, Sbox []byte) []byte {
	sbox := make([]byte, len(Sbox))
	copy(sbox, Sbox)

	v55 := 0
	v56 := 0
	v57 := 0

	for {
		v59 := (v56 + 1) & 0xFF
		temp := (v55 ^ int(sbox[v59])) + 2*(v55&int(sbox[v59]))
		v62 := temp & 0xFF
		v63 := sbox[v62]
		sbox[v59] = v63
		sbox[v62] = v63

		index := (int(sbox[v59]) | int(v63)) + (int(sbox[v59]) & int(v63))
		index &= 0xFF

		data[v57] ^= sbox[index]

		v57 = 2*(v57&1) + (v57 ^ 1)
		v55 = v62
		v56 = v59

		if v57 >= keyLen {
			break
		}
		if v57 == 0 {
			v55 = 0
			v56 = 0
			v57 = 0
		}
	}

	return data
}

// makeGorgonLast 最后一步处理
func makeGorgonLast(data []byte) string {
	res := ""
	for i := 0; i < len(data); i++ {
		nextByte := data[0]
		if i != len(data)-1 {
			nextByte = data[i+1]
		}
		data[i] = ((data[i] >> 4) | (data[i] << 4)) ^ nextByte

		tem1 := (int(data[i]) << 1) & 0xffaa
		tem2 := (int(data[i]) >> 1) & 0x55
		tem3 := tem1 | tem2
		tem4 := ((tem3 << 2) & 0xffffcf) | ((tem3 >> 2) & 0x33)
		tem5 := (tem4 >> 4) & 0xf

		mask := (1 << 28) - 1
		lsb := 4
		ans := (tem5 & ^(mask << lsb)) | ((tem4 & mask) << lsb)
		ans = (ans ^ 0xffffffeb) & 0xff
		data[i] = byte(ans)

		res += hex.EncodeToString([]byte{byte(ans)})
	}
	return res
}

// MakeGorgon 生成X-Gorgon签名
// Python: make_gorgon(khronos:str="1751607382", ...)
// Python: hex(int(khronos)).split("0x")[1] - 将十进制字符串转成整数，再转成十六进制（去掉0x前缀）
func MakeGorgon(khronos, queryString, xSSStub, sdkVersion string) string {
	if sdkVersion == "" {
		sdkVersion = "0000000020020205"
	}

	key := "4a0016a8476c0080"
	Sbox := makeGorgonRc4Init(key)

	md5Query := md5HashString(queryString)[:8]

	xSSStubData := xSSStub[:8]
	if xSSStub == "" || xSSStub == "00000000000000000000000000000000" {
		xSSStubData = "00000000"
	}

	// Python: hex(int(khronos)).split("0x")[1]
	var khronosHex string
	var ts int64
	if _, err := fmt.Sscanf(khronos, "%d", &ts); err == nil {
		khronosHex = fmt.Sprintf("%x", ts)
	} else {
		khronosHex = khronos // 如果已经是十六进制，直接使用
	}

	dataHex := md5Query + xSSStubData + sdkVersion + khronosHex
	data, _ := hex.DecodeString(dataHex)
	keyLen := len(data)

	data = makeGorgonRc4(data, keyLen, Sbox)
	lastTwenty := makeGorgonLast(data)

	return "840480a80000" + lastTwenty
}
