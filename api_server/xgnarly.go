package main

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// Constants
var (
	aa = []uint32{
		0xFFFFFFFF, 138, 1498001188, 211147047, 253, 0, 203, 288, 9,
		1196819126, 3212677781, 135, 263, 193, 58, 18, 244, 2931180889, 240, 173,
		268, 2157053261, 261, 175, 14, 5, 171, 270, 156, 258, 13, 15, 3732962506,
		185, 169, 2, 6, 132, 162, 200, 3, 160, 217618912, 62, 2517678443, 44, 164,
		4, 96, 183, 2903579748, 3863347763, 119, 181, 10, 190, 8, 2654435769, 259,
		104, 230, 128, 2633865432, 225, 1, 257, 143, 179, 16, 600974999, 185100057,
		32, 188, 53, 2718276124, 177, 196, 0xFFFFFFFF, 147, 117, 17, 49, 7, 28, 12, // 4294967296 overflows uint32, use 0xFFFFFFFF (4294967295) for randomInt range
		266, 216, 11, 0, 45, 166, 247, 1451689750,
	}
	Ot = []uint32{aa[9], aa[69], aa[51], aa[92]} // constants prepended to ChaCha key
)

const MASK32 = 0xFFFFFFFF

// Global state (thread-safe with mutex for concurrent access)
var (
	ktMutex sync.Mutex
	kt      []uint32 // 16-word ChaCha state
	St      int      // position pointer (starts at 0)
)

// initPrngState initializes PRNG state (faithful clone of JS impl)
func initPrngState(timestampMs int64) []uint32 {
	nowMs := uint32(timestampMs)
	state := make([]uint32, 16)
	state[0] = aa[44]
	state[1] = aa[74]
	state[2] = aa[10]
	state[3] = aa[62]
	state[4] = aa[42]
	state[5] = aa[17]
	state[6] = aa[2]
	state[7] = aa[21]
	state[8] = aa[3]
	state[9] = aa[70]
	state[10] = aa[50]
	state[11] = aa[32]
	state[12] = (aa[0] & nowMs) & MASK32

	// crypto.randomInt equivalent - use crypto/rand to match JS behavior
	max := big.NewInt(int64(aa[77]))

	r1, _ := rand.Int(rand.Reader, max)
	state[13] = uint32(r1.Int64())

	r2, _ := rand.Int(rand.Reader, max)
	state[14] = uint32(r2.Int64())

	r3, _ := rand.Int(rand.Reader, max)
	state[15] = uint32(r3.Int64())

	return state
}

func init() {
	// Initialize state - persists across calls (matches JS behavior)
	kt = initPrngState(time.Now().UnixMilli())
	St = int(aa[88])
}

// ResetPrngState resets the PRNG state (useful for testing)
func ResetPrngState(timestampMs int64) {
	ktMutex.Lock()
	defer ktMutex.Unlock()
	kt = initPrngState(timestampMs)
	St = int(aa[88])
}

// Bit-twiddling helpers
func u32(x uint32) uint32 {
	return x & MASK32
}

func rotl(x uint32, n int) uint32 {
	return u32((x << n) | (x >> (32 - n)))
}

// ChaCha core
func quarter(st []uint32, a, b, c, d int) {
	st[a] = u32(st[a] + st[b])
	st[d] = rotl(st[d]^st[a], 16)
	st[c] = u32(st[c] + st[d])
	st[b] = rotl(st[b]^st[c], 12)
	st[a] = u32(st[a] + st[b])
	st[d] = rotl(st[d]^st[a], 8)
	st[c] = u32(st[c] + st[d])
	st[b] = rotl(st[b]^st[c], 7)
}

func chachaBlock(state []uint32, rounds int) []uint32 {
	w := make([]uint32, 16)
	copy(w, state)
	r := 0
	for r < rounds {
		// column round
		quarter(w, 0, 4, 8, 12)
		quarter(w, 1, 5, 9, 13)
		quarter(w, 2, 6, 10, 14)
		quarter(w, 3, 7, 11, 15)
		r++
		if r >= rounds {
			break
		}
		// diagonal round
		quarter(w, 0, 5, 10, 15)
		quarter(w, 1, 6, 11, 12)
		quarter(w, 2, 7, 12, 13)
		quarter(w, 3, 4, 13, 14)
		r++
	}
	for i := 0; i < 16; i++ {
		w[i] = u32(w[i] + state[i])
	}
	return w
}

func bumpCounter(st []uint32) {
	st[12] = u32(st[12] + 1)
}

// JS-faithful PRNG (rand) - thread-safe version
func randXgnarly() float64 {
	ktMutex.Lock()
	defer ktMutex.Unlock()

	e := chachaBlock(kt, 8) // 8 double-rounds
	t := e[St]
	r := (e[St+8] & 0xFFFFFFF0) >> 11
	if St == 7 {
		bumpCounter(kt)
		St = 0
	} else {
		St++
	}
	// Use exact constant to match JavaScript 2**53
	const twoPow53 = 9007199254740992.0
	return (float64(t) + 4294967296.0*float64(r)) / twoPow53
}

// Utilities
func numToBytes(val uint32) []byte {
	if val < 255*255 {
		return []byte{byte((val >> 8) & 0xFF), byte(val & 0xFF)}
	}
	return []byte{
		byte((val >> 24) & 0xFF),
		byte((val >> 16) & 0xFF),
		byte((val >> 8) & 0xFF),
		byte(val & 0xFF),
	}
}

func beIntFromStr(str string) uint32 {
	bytes := []byte(str)
	if len(bytes) > 4 {
		bytes = bytes[:4]
	}
	var acc uint32
	for _, b := range bytes {
		acc = (acc << 8) | uint32(b)
	}
	return acc & MASK32
}

// Message encryption (Ab21 in original)
func encryptChaCha(keyWords []uint32, rounds int, bytes []byte) {
	// pack to 32-bit words, little-endian
	nFull := len(bytes) / 4
	leftover := len(bytes) % 4
	words := make([]uint32, (len(bytes)+3)/4)

	for i := 0; i < nFull; i++ {
		j := 4 * i
		words[i] = uint32(bytes[j]) |
			(uint32(bytes[j+1]) << 8) |
			(uint32(bytes[j+2]) << 16) |
			(uint32(bytes[j+3]) << 24)
	}
	if leftover > 0 {
		var v uint32
		base := 4 * nFull
		for c := 0; c < leftover; c++ {
			v |= uint32(bytes[base+c]) << (8 * c)
		}
		words[nFull] = v
	}

	// XOR with ChaCha stream
	o := 0
	state := make([]uint32, len(keyWords))
	copy(state, keyWords)
	for o+16 < len(words) {
		stream := chachaBlock(state, rounds)
		bumpCounter(state)
		for k := 0; k < 16; k++ {
			words[o+k] ^= stream[k]
		}
		o += 16
	}
	remain := len(words) - o
	stream := chachaBlock(state, rounds)
	for k := 0; k < remain; k++ {
		words[o+k] ^= stream[k]
	}

	// flatten back to bytes
	for i := 0; i < nFull; i++ {
		w := words[i]
		j := 4 * i
		bytes[j] = byte(w & 0xFF)
		bytes[j+1] = byte((w >> 8) & 0xFF)
		bytes[j+2] = byte((w >> 16) & 0xFF)
		bytes[j+3] = byte((w >> 24) & 0xFF)
	}
	if leftover > 0 {
		w := words[nFull]
		base := 4 * nFull
		for c := 0; c < leftover; c++ {
			bytes[base+c] = byte((w >> (8 * c)) & 0xFF)
		}
	}
}

// Ab22 helper: prepend Ot, encrypt, return string
func Ab22(key12Words []uint32, rounds int, str string) string {
	state := make([]uint32, 16)
	copy(state, Ot)
	copy(state[4:], key12Words)
	data := []byte(str)
	encryptChaCha(state, rounds, data)
	return string(data)
}

// Encrypt is the main API
// queryString: query string
// body: POST body
// userAgent: user agent string
// envcode: default 0
// version: "5.1.0" or "5.1.1", default "5.1.1"
// timestampMs: override of time.Now().UnixMilli()
// Returns: encoded token
func EncryptXgnarly(queryString, body, userAgent string, envcode int, version string, timestampMs int64) (string, error) {
	if version == "" {
		version = "5.1.1"
	}
	if timestampMs == 0 {
		timestampMs = time.Now().UnixMilli()
	}

	// Reset PRNG state for deterministic output (matches JavaScript version)
	// (Reset removed to match JS global state persistence)

	// build the obj map with insertion order intact (use slice to maintain order)
	type kvPair struct {
		key   int
		value interface{}
	}
	objPairs := []kvPair{}
	objPairs = append(objPairs, kvPair{1, 1})
	objPairs = append(objPairs, kvPair{2, envcode})

	// MD5 hashes
	hash1 := md5.Sum([]byte(queryString))
	objPairs = append(objPairs, kvPair{3, fmt.Sprintf("%x", hash1)})

	hash2 := md5.Sum([]byte(body))
	objPairs = append(objPairs, kvPair{4, fmt.Sprintf("%x", hash2)})

	hash3 := md5.Sum([]byte(userAgent))
	objPairs = append(objPairs, kvPair{5, fmt.Sprintf("%x", hash3)})

	objPairs = append(objPairs, kvPair{6, int(timestampMs / 1000)})
	objPairs = append(objPairs, kvPair{7, 1508145731})
	objPairs = append(objPairs, kvPair{8, int((timestampMs * 1000) % 2147483648)})
	objPairs = append(objPairs, kvPair{9, version})

	if version == "5.1.1" {
		objPairs = append(objPairs, kvPair{10, "1.0.0.314"})
		objPairs = append(objPairs, kvPair{11, 1})
		var v12 uint32
		for i := 0; i < 11; i++ {
			v := objPairs[i].value
			var toXor uint32
			switch val := v.(type) {
			case int:
				toXor = uint32(val)
			case string:
				toXor = beIntFromStr(val)
			default:
				return "", fmt.Errorf("unexpected type for key %d", objPairs[i].key)
			}
			v12 ^= toXor
		}
		objPairs = append(objPairs, kvPair{12, int(v12 & MASK32)})
	} else if version != "5.1.0" {
		return "", fmt.Errorf("unsupported version: %s", version)
	}

	// compute v0 after 12 (Python order) - only XOR numbers
	// Note: In JS, it iterates from 1 to obj.size (which is 12 at this point)
	// So we iterate through pairs 0-11 (which correspond to keys 1-12)
	var v0 uint32
	for i := 0; i < len(objPairs); i++ {
		if num, ok := objPairs[i].value.(int); ok {
			v0 ^= uint32(num)
		}
	}
	// Append v0 at the end (key 0) - this matches JS: obj.set(0, v0 >>> 0)
	// In JS Map, the last set() call means key 0 will be iterated last
	objPairs = append(objPairs, kvPair{0, int(v0 & MASK32)})

	// serialize payload - iterate in order
	payload := []byte{}
	payload = append(payload, byte(len(objPairs))) // count byte
	for _, pair := range objPairs {
		payload = append(payload, byte(pair.key))
		var valBytes []byte
		switch val := pair.value.(type) {
		case int:
			valBytes = numToBytes(uint32(val))
		case string:
			valBytes = []byte(val)
		default:
			return "", fmt.Errorf("unexpected type for value")
		}
		payload = append(payload, numToBytes(uint32(len(valBytes)))...)
		payload = append(payload, valBytes...)
	}
	baseStr := string(payload)

	// generate 12 random key words
	keyWords := make([]uint32, 12)
	keyBytes := []byte{}
	roundAccum := 0
	for i := 0; i < 12; i++ {
		rnd := randXgnarly()
		word := uint32(rnd*4294967296.0) & MASK32 // 2^32 * rnd
		keyWords[i] = word
		roundAccum = (roundAccum + int(word&15)) & 15
		keyBytes = append(keyBytes,
			byte(word&0xFF),
			byte((word>>8)&0xFF),
			byte((word>>16)&0xFF),
			byte((word>>24)&0xFF)) // little-endian
	}
	rounds := roundAccum + 5

	// encrypt baseStr
	enc := Ab22(keyWords, rounds, baseStr)

	// splice keyBytes into enc at computed insertPos
	insertPos := 0
	for _, b := range keyBytes {
		insertPos = (insertPos + int(b)) % (len(enc) + 1)
	}
	for i := 0; i < len(enc); i++ {
		insertPos = (insertPos + int(enc[i])) % (len(enc) + 1)
	}

	keyBytesStr := string(keyBytes)
	finalStr := string(((1<<6)^(1<<3)^3)&0xFF) + // constant 'K'
		enc[:insertPos] +
		keyBytesStr +
		enc[insertPos:]

	// custom alphabet Base-64
	alphabet := "u09tbS3UvgDEe6r-ZVMXzLpsAohTn7mdINQlW412GqBjfYiyk8JORCF5/xKHwacP="
	out := []byte{}
	fullLen := (len(finalStr) / 3) * 3
	for i := 0; i < fullLen; i += 3 {
		block := (uint32(finalStr[i]) << 16) |
			(uint32(finalStr[i+1]) << 8) |
			uint32(finalStr[i+2])
		out = append(out,
			alphabet[(block>>18)&63],
			alphabet[(block>>12)&63],
			alphabet[(block>>6)&63],
			alphabet[block&63])
	}
	// Handle padding if needed (not in JS version, but for completeness)
	remainder := len(finalStr) - fullLen
	if remainder > 0 {
		// This case is not handled in JS version, but we should handle it
		// For now, just return what we have
	}
	return string(out), nil
}
