package main

import (
	"crypto/md5"
	"encoding/base64"
)

// Constants
const (
	STANDARD_B64_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	CUSTOM_B64_ALPHABET   = "Dkdpgh4ZKsQB80/Mfvw36XI1R25-WUAlEi7NLboqYTOPuzmFjJnryx9HVGcaStCe"
)

// Build translation map
var encTrans map[byte]byte

func init() {
	encTrans = make(map[byte]byte)
	for i := 0; i < len(STANDARD_B64_ALPHABET); i++ {
		encTrans[STANDARD_B64_ALPHABET[i]] = CUSTOM_B64_ALPHABET[i]
	}
}

// customB64Encode performs custom Base64 encoding
func customB64Encode(data []byte) string {
	stdB64 := base64.StdEncoding.EncodeToString(data)
	result := make([]byte, len(stdB64))
	for i := 0; i < len(stdB64); i++ {
		if trans, ok := encTrans[stdB64[i]]; ok {
			result[i] = trans
		} else {
			result[i] = stdB64[i] // '=' / newlines pass through
		}
	}
	return string(result)
}

// stdMd5Enc performs MD5 hash
func stdMd5Enc(data []byte) []byte {
	hash := md5.Sum(data)
	return hash[:]
}

// rc4Enc performs RC4 encryption (KSA + PRGA)
func rc4Enc(keyBuf, plaintextBuf []byte) []byte {
	s := make([]byte, 256)
	for i := 0; i < 256; i++ {
		s[i] = byte(i)
	}

	j := 0
	keyLen := len(keyBuf)
	for i := 0; i < 256; i++ {
		j = (j + int(s[i]) + int(keyBuf[i%keyLen])) & 0xff
		s[i], s[j] = s[j], s[i]
	}

	out := make([]byte, len(plaintextBuf))
	i := 0
	j = 0
	for n := 0; n < len(plaintextBuf); n++ {
		i = (i + 1) & 0xff
		j = (j + int(s[i])) & 0xff
		s[i], s[j] = s[j], s[i]
		k := s[(s[i]+s[j])&0xff]
		out[n] = plaintextBuf[n] ^ k
	}
	return out
}

// xorKey computes XOR of all bytes
func xorKey(buf []byte) byte {
	result := byte(0)
	for _, b := range buf {
		result ^= b
	}
	return result
}

// Encrypt replicates encrypt(params, postData, userAgent, timestamp) from JavaScript
// params: query string
// postData: POST body
// userAgent: user agent string
// timestamp: Unix-epoch seconds (unsigned 32-bit)
// Returns: custom-Base-64 signature
func Encrypt(params, postData, userAgent string, timestamp uint32) string {
	uaKey := []byte{0x00, 0x01, 0x0e}
	listKey := []byte{0xff}
	fixedVal := uint32(0x4a41279f) // 1245249439

	// double-MD5s
	md5Params := stdMd5Enc(stdMd5Enc([]byte(params)))
	md5Post := stdMd5Enc(stdMd5Enc([]byte(postData)))

	// UA → RC4 → Base64 → MD5
	uaRc4 := rc4Enc(uaKey, []byte(userAgent))
	uaB64 := base64.StdEncoding.EncodeToString(uaRc4)
	md5Ua := stdMd5Enc([]byte(uaB64))

	// build buffer exactly like JavaScript
	parts := [][]byte{
		{0x40},                    // literal 64
		uaKey,                     // [0x00, 0x01, 0x0e]
		md5Params[14:16],          // last 2 bytes
		md5Post[14:16],           // last 2 bytes
		md5Ua[14:16],             // last 2 bytes
		uint32ToBytesBE(timestamp),
		uint32ToBytesBE(fixedVal),
	}

	buffer := concatBytes(parts) // 18 bytes (so far)

	// append checksum safely
	checksum := xorKey(buffer)
	buffer = append(buffer, checksum) // now 19 bytes

	// final wrapper
	enc := concatBytes([][]byte{
		{0x02},
		listKey,
		rc4Enc(listKey, buffer),
	})

	return customB64Encode(enc)
}

// Helper functions
func uint32ToBytesBE(val uint32) []byte {
	return []byte{
		byte(val >> 24),
		byte(val >> 16),
		byte(val >> 8),
		byte(val),
	}
}

func concatBytes(parts [][]byte) []byte {
	totalLen := 0
	for _, part := range parts {
		totalLen += len(part)
	}
	result := make([]byte, 0, totalLen)
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}

