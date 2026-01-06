package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"strconv"
	"time"
)

// EncryptPassword4NodeCompatible 完全兼容 Node.js encryptPassword4
func EncryptPassword4NodeCompatible(pubKeyRaw string, keyId int, password string) (string, string, error) {
	if pubKeyRaw == "" {
		return "", "", fmt.Errorf("public key is empty")
	}

	// 1️⃣ 随机 AES key (32 bytes) 和 IV (12 bytes)
	randKey := make([]byte, 32)
	if _, err := rand.Read(randKey); err != nil {
		return "", "", err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return "", "", err
	}

	// 2️⃣ 解析 RSA 公钥
	var pubKeyBytes []byte
	if block, _ := pem.Decode([]byte(pubKeyRaw)); block != nil {
		pubKeyBytes = block.Bytes
	} else {
		decoded, err := base64.StdEncoding.DecodeString(pubKeyRaw)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode public key as base64: %v", err)
		}
		if block2, _ := pem.Decode(decoded); block2 != nil {
			pubKeyBytes = block2.Bytes
		} else {
			pubKeyBytes = decoded
		}
	}

	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		pub, err = x509.ParsePKCS1PublicKey(pubKeyBytes)
		if err != nil {
			return "", "", fmt.Errorf("failed to parse public key: %v", err)
		}
	}
	rsaPubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", "", fmt.Errorf("not a valid RSA public key")
	}

	// 3️⃣ RSA 加密 AES key
	rsaEncrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPubKey, randKey)
	if err != nil {
		return "", "", err
	}

	// 4️⃣ AES-256-GCM 加密密码
	timeStr := strconv.FormatInt(time.Now().Unix(), 10) // 秒级时间戳
	block, err := aes.NewCipher(randKey)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 12)
	if err != nil {
		return "", "", err
	}

	// Go 的 Seal 返回 [密文][16 byte Tag]
	sealed := gcm.Seal(nil, iv, []byte(password), []byte(timeStr))
	aesEncrypted := sealed[:len(sealed)-16]
	authTag := sealed[len(sealed)-16:]

	// 5️⃣ 拼接二进制数据
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(1)                                                  // Version
	buf.WriteByte(byte(keyId))                                        // Key ID
	buf.Write(iv)                                                     // IV
	binary.Write(buf, binary.LittleEndian, uint16(len(rsaEncrypted))) // RSA len
	buf.Write(rsaEncrypted)                                           // RSA Encrypted key
	buf.Write(authTag)                                                // Auth tag
	buf.Write(aesEncrypted)                                           // AES 密文

	// 6️⃣ Base64 + 前缀
	result := "#PWD_INSTAGRAM:4:" + base64.StdEncoding.EncodeToString(buf.Bytes())
	return result, timeStr, nil
}
