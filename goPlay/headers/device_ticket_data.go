package headers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"
)

// DeltaKeyPair 密钥对结构
type DeltaKeyPair struct {
	PrivKeyHex string
	PubKeyB64  string
	PrivKey    *ecdsa.PrivateKey
}

// GenerateDeltaKeypair 生成ECDSA密钥对 - 完全按照Python的generate_delta_keypair实现
func GenerateDeltaKeypair() (*DeltaKeyPair, error) {
	// 使用P-256曲线 (secp256r1)
	curve := elliptic.P256()

	privKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}

	// 私钥整数 → 32 字节 big-endian → hex
	dInt := privKey.D
	privBytes := dInt.Bytes()
	// 确保32字节
	if len(privBytes) < 32 {
		padding := make([]byte, 32-len(privBytes))
		privBytes = append(padding, privBytes...)
	}
	privKeyHex := hex.EncodeToString(privBytes)

	// 公钥未压缩点：04 || X(32) || Y(32)
	xBytes := privKey.PublicKey.X.Bytes()
	yBytes := privKey.PublicKey.Y.Bytes()
	// 确保32字节
	if len(xBytes) < 32 {
		padding := make([]byte, 32-len(xBytes))
		xBytes = append(padding, xBytes...)
	}
	if len(yBytes) < 32 {
		padding := make([]byte, 32-len(yBytes))
		yBytes = append(padding, yBytes...)
	}
	uncompressed := append([]byte{0x04}, xBytes...)
	uncompressed = append(uncompressed, yBytes...)

	// Base64编码
	ttPublicKeyB64 := base64.StdEncoding.EncodeToString(uncompressed)

	return &DeltaKeyPair{
		PrivKeyHex: privKeyHex,
		PubKeyB64:  ttPublicKeyB64,
		PrivKey:    privKey,
	}, nil
}

// LoadKeypairFromPrivHex 从私钥hex加载密钥对 - 完全按照Python的load_keypair_from_priv_hex实现
func LoadKeypairFromPrivHex(privKeyHex string) (*DeltaKeyPair, error) {
	curve := elliptic.P256()

	privBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return nil, err
	}

	// 从big-endian字节转换为整数
	dInt := new(big.Int).SetBytes(privBytes)

	// 使用ScalarBaseMult计算公钥
	x, y := curve.ScalarBaseMult(dInt.Bytes())

	privKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: dInt,
	}

	// 公钥未压缩点：04 || X(32) || Y(32)
	xBytes := x.Bytes()
	yBytes := y.Bytes()
	// 确保32字节
	if len(xBytes) < 32 {
		padding := make([]byte, 32-len(xBytes))
		xBytes = append(padding, xBytes...)
	}
	if len(yBytes) < 32 {
		padding := make([]byte, 32-len(yBytes))
		yBytes = append(padding, yBytes...)
	}
	uncompressed := append([]byte{0x04}, xBytes...)
	uncompressed = append(uncompressed, yBytes...)

	ttPublicKeyB64 := base64.StdEncoding.EncodeToString(uncompressed)

	return &DeltaKeyPair{
		PrivKeyHex: privKeyHex,
		PubKeyB64:  ttPublicKeyB64,
		PrivKey:    privKey,
	}, nil
}

// deltaSign 使用ECDSA签名数据 - 完全按照Python的delta_sign实现
func deltaSign(unsigned string, privKey *ecdsa.PrivateKey) (string, error) {
	// 1) 先手动 SHA256
	hash := sha256.Sum256([]byte(unsigned))
	digest := hash[:]

	// 2) 用 ECDSA 签名
	r, s, err := ecdsa.Sign(rand.Reader, privKey, digest)
	if err != nil {
		return "", err
	}

	// 3) DER编码签名
	derSig := marshalDERSignature(r, s)

	// 4) Base64编码
	return base64.StdEncoding.EncodeToString(derSig), nil
}

// marshalDERSignature 将r,s编码为DER格式
func marshalDERSignature(r, s *big.Int) []byte {
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// 如果最高位为1，需要补0
	if len(rBytes) > 0 && rBytes[0]&0x80 != 0 {
		rBytes = append([]byte{0}, rBytes...)
	}
	if len(sBytes) > 0 && sBytes[0]&0x80 != 0 {
		sBytes = append([]byte{0}, sBytes...)
	}

	// DER格式: 0x30 + 总长度 + 0x02 + r长度 + r + 0x02 + s长度 + s
	totalLen := 2 + len(rBytes) + 2 + len(sBytes)
	result := make([]byte, 0, totalLen+2)
	result = append(result, 0x30, byte(totalLen))
	result = append(result, 0x02, byte(len(rBytes)))
	result = append(result, rBytes...)
	result = append(result, 0x02, byte(len(sBytes)))
	result = append(result, sBytes...)

	return result
}

// BuildGuard 构建设备守护或票据守护数据 - 完全按照Python的build_guard实现
// deviceGuardData0: 如果是设备相关的话，直接传服务器返回的这个内容（map[string]interface{}）
// cookie: 如果是ticket相关的话，直接传cookie进来（map[string]string）
// path: 路径，默认为 "/aweme/v1/aweme/stats/"
// timestamp: 时间戳，如果为0则使用当前时间
// privHex: 私钥hex，如果为空则生成新的
// isTicket: 是否是ticket模式
// 返回: headers字典
func BuildGuard(
	deviceGuardData0 map[string]interface{},
	cookie map[string]string,
	path string,
	timestamp int64,
	privHex string,
	isTicket bool,
) (map[string]string, error) {
	if !isTicket {
		// 设备守护模式
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}

		// 1) 拿 keypair：优先用你给的 priv_hex，否则就现生成一对
		var kp *DeltaKeyPair
		var err error
		if privHex != "" && privHex != "..." {
			kp, err = LoadKeypairFromPrivHex(privHex)
		} else {
			kp, err = GenerateDeltaKeypair()
		}
		if err != nil {
			return nil, fmt.Errorf("生成密钥对失败: %w", err)
		}

		// 获取device_token
		deviceToken, ok := deviceGuardData0["device_token"].(string)
		if !ok {
			return nil, fmt.Errorf("device_token not found in device_guard_data0")
		}

		// 2) unsigned 字符串 —— 注意这里要和真实请求完全一样
		unsigned := fmt.Sprintf("device_token=%s&path=%s&timestamp=%d", deviceToken, path, timestamp)

		// 3) 签名
		dreqSignB64, err := deltaSign(unsigned, kp.PrivKey)
		if err != nil {
			return nil, fmt.Errorf("签名失败: %w", err)
		}

		// 构建device_guard_data1
		deviceGuardData1 := map[string]interface{}{
			"device_token": deviceGuardData0["device_token"],
			"timestamp":    timestamp,
			"req_content":  "device_token,path,timestamp",
			"dtoken_sign":  deviceGuardData0["dtoken_sign"],
			"dreq_sign":    dreqSignB64,
		}

		// JSON编码（无空格，与Python的separators=(',', ':')一致）
		deviceGuardData1JSON, err := json.Marshal(deviceGuardData1)
		if err != nil {
			return nil, fmt.Errorf("JSON编码失败: %w", err)
		}

		// Base64编码
		ttDeviceGuardClientData := base64.StdEncoding.EncodeToString(deviceGuardData1JSON)

		// 4) 你要塞进 header 的东西
		headers := map[string]string{
			"tt-device-guard-iteration-version": "1",
			"tt-ticket-guard-public-key":        kp.PubKeyB64,
			"tt-ticket-guard-version":           "3",
			"tt-device-guard-client-data":       ttDeviceGuardClientData,
		}

		return headers, nil
	} else {
		// 票据守护模式
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}

		// 获取cookie中的值
		xTtToken := ""
		if cookie != nil {
			xTtToken = cookie["x-tt-token"]
			if xTtToken == "" {
				xTtToken = ""
			}
		}

		tsSign := ""
		if cookie != nil {
			tsSign = cookie["ts_sign"]
			if tsSign == "" {
				tsSign = cookie["ts_sign_ree"]
			}
		}

		// 构建unsigned字符串
		unsigned := fmt.Sprintf("%s&path=%s&timestamp=%d", xTtToken, path, int(time.Now().Unix()))

		// 获取keypair
		var kp *DeltaKeyPair
		var err error
		if privHex != "" && privHex != "..." {
			kp, err = LoadKeypairFromPrivHex(privHex)
		} else {
			kp, err = GenerateDeltaKeypair()
		}
		if err != nil {
			return nil, fmt.Errorf("生成密钥对失败: %w", err)
		}

		// 签名
		reqSign, err := deltaSign(unsigned, kp.PrivKey)
		if err != nil {
			return nil, fmt.Errorf("签名失败: %w", err)
		}

		// 构建ticket_guard_data
		ticketGuardData := map[string]interface{}{
			"req_content": "ticket,path,timestamp",
			"req_sign":    reqSign,
			"timestamp":   timestamp,
			"ts_sign":     tsSign,
		}

		// JSON编码（无空格）
		ticketGuardDataJSON, err := json.Marshal(ticketGuardData)
		if err != nil {
			return nil, fmt.Errorf("JSON编码失败: %w", err)
		}

		// Base64编码
		ttTicketGuardClientData := base64.StdEncoding.EncodeToString(ticketGuardDataJSON)

		// 构建headers
		headers := map[string]string{
			"tt-ticket-guard-client-data":       ttTicketGuardClientData,
			"tt-ticket-guard-iteration-version": "0",
			"tt-ticket-guard-public-key":        kp.PubKeyB64,
			"tt-ticket-guard-version":           "3",
		}

		return headers, nil
	}
}

// DeltaSign 使用ECDSA签名数据（公开接口，用于向后兼容）
func DeltaSign(kp *DeltaKeyPair, data []byte) (string, error) {
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, kp.PrivKey, hash[:])
	if err != nil {
		return "", err
	}

	// DER编码签名
	signature := marshalDERSignature(r, s)
	return base64.StdEncoding.EncodeToString(signature), nil
}

// BuildGuardLegacy 向后兼容的BuildGuard接口（旧版本，用于stats.go等文件）
// 这个函数保持与旧代码的兼容性
// 参数: kp *DeltaKeyPair, isDevice bool, payload map[string]interface{}
// 返回: *GuardResult
// 注意：这个函数与Python版本不一致，仅用于向后兼容
func BuildGuardLegacy(kp *DeltaKeyPair, isDevice bool, payload map[string]interface{}) (*GuardResult, error) {
	// 添加时间戳
	if payload == nil {
		payload = make(map[string]interface{})
	}
	payload["timestamp"] = time.Now().UnixMilli()

	// JSON编码
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Base64编码
	payloadB64 := base64.StdEncoding.EncodeToString(payloadBytes)

	// 签名
	signature, err := DeltaSign(kp, payloadBytes)
	if err != nil {
		return nil, err
	}

	// 构建客户端数据
	clientData := fmt.Sprintf("%s.%s", payloadB64, signature)

	return &GuardResult{
		PublicKey:  kp.PubKeyB64,
		ClientData: clientData,
	}, nil
}

// GuardResult 守护结果结构（用于向后兼容）
type GuardResult struct {
	PublicKey  string
	ClientData string
}

// MakeDeviceTicketData 生成设备票据数据（用于向后兼容）
func MakeDeviceTicketData(privKeyHex string, deviceID string, installID string) (map[string]string, error) {
	var kp *DeltaKeyPair
	var err error

	if privKeyHex != "" {
		kp, err = LoadKeypairFromPrivHex(privKeyHex)
	} else {
		kp, err = GenerateDeltaKeypair()
	}
	if err != nil {
		return nil, err
	}

	deviceGuardData0 := map[string]interface{}{
		"device_token": "",
		"dtoken_sign":  "",
	}

	headers, err := BuildGuard(deviceGuardData0, nil, "/aweme/v1/aweme/stats/", 0, privKeyHex, false)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"tt-device-guard-public-key":  headers["tt-ticket-guard-public-key"],
		"tt-device-guard-client-data": headers["tt-device-guard-client-data"],
		"private_key_hex":             kp.PrivKeyHex,
	}, nil
}
