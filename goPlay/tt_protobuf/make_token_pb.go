package tt_protobuf

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
)

// GenerateFakeMediadrmID 生成伪造的MediaDRM ID
func GenerateFakeMediadrmID() string {
	return randomBase64(32)
}

// GenerateRandomAPKPath 生成随机APK路径
func GenerateRandomAPKPath() string {
	part1 := randomBase64(16)
	part2 := randomBase64(16)
	return fmt.Sprintf("/data/app/~~%s/com.zhiliaoapp.musically-%s/base.apk", part1, part2)
}

func generateRandomUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff)
}

func randomBase64(numBytes int) string {
	buf := make([]byte, numBytes)
	if _, err := crand.Read(buf); err != nil {
		rand.Read(buf)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func randomHexString(numBytes int) string {
	buf := make([]byte, numBytes)
	if _, err := crand.Read(buf); err != nil {
		rand.Read(buf)
	}
	return hex.EncodeToString(buf)
}

// EncodeTokenEncryptOneOne 编码TokenEncrypt_One_One消息
func EncodeTokenEncryptOneOne(t *TokenEncrypt_One_One) []byte {
	if t == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteFixed64(1, t.Unknown1)
	return e.Bytes()
}

// EncodeTokenEncryptTwo 编码TokenEncrypt_Two消息
func EncodeTokenEncryptTwo(t *TokenEncrypt_Two) []byte {
	if t == nil {
		return nil
	}
	e := NewProtobufEncoder()
	for _, v := range t.S6 {
		if v == 0 {
			continue
		}
		e.WriteInt64(6, int64(v))
	}
	return e.Bytes()
}

// EncodeTokenEncryptOne 编码TokenEncrypt_One消息
func EncodeTokenEncryptOne(t *TokenEncrypt_One) []byte {
	if t == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, t.Notset1)
	e.WriteString(2, t.Changshang)
	e.WriteString(3, t.Xinghao)
	e.WriteString(4, t.Notset2)
	e.WriteString(5, t.OS)
	e.WriteString(6, t.OSVersion)
	if t.TokenEncryptOneOne != nil {
		e.WriteMessage(7, EncodeTokenEncryptOneOne(t.TokenEncryptOneOne))
	}
	if t.DensityDPI != 0 {
		e.WriteInt32(8, int32(t.DensityDPI))
	}
	e.WriteString(9, t.BuildID)
	e.WriteInt64(10, int64(t.OSBuildTime))
	e.WriteString(11, t.AppLanguage)
	e.WriteString(12, t.TimeZone)
	e.WriteInt64(13, int64(t.Unknown2))
	e.WriteInt64(14, int64(t.Unknown3))
	e.WriteInt64(15, int64(t.Unknown4))
	e.WriteInt64(16, int64(t.Stable1))
	e.WriteInt64(17, int64(t.Stable2))
	e.WriteInt64(18, int64(t.Unknown5))
	e.WriteString(19, t.Notset3)
	e.WriteString(20, t.Notset4)
	e.WriteString(21, t.AndroidID)
	e.WriteString(22, t.Notset5)
	e.WriteString(23, t.Notset6)
	e.WriteString(24, t.MediaDrm)
	e.WriteInt64(25, int64(t.LaunchTime))
	e.WriteString(26, t.BootID)
	e.WriteInt64(27, int64(t.Unknown6))
	e.WriteString(28, t.Notset7)
	e.WriteInt64(29, int64(t.Stable3))
	e.WriteInt64(30, int64(t.Stable4))
	e.WriteString(31, t.Notset8)
	e.WriteString(32, t.DefaultGateway)
	e.WriteString(33, t.IPDNS)
	e.WriteString(34, t.Notset9)
	e.WriteString(35, t.Netset10)
	e.WriteInt64(36, int64(t.Stable5))
	e.WriteInt64(37, int64(t.Stable6))
	e.WriteString(38, t.Notset11)
	e.WriteString(39, t.Notset12)
	e.WriteString(40, t.IPArray)
	e.WriteInt64(41, int64(t.Stable7))
	e.WriteInt64(42, int64(t.Stable8))
	e.WriteString(44, t.Notset13)
	e.WriteInt64(45, int64(t.ExpiredTime))
	e.WriteInt64(46, int64(t.SendTime))
	e.WriteString(47, t.InstallPath)
	e.WriteInt64(48, int64(t.OSAPI))
	e.WriteInt64(49, int64(t.Stable9))
	e.WriteString(50, t.Notset14)
	e.WriteInt64(51, int64(t.Stable10))
	e.WriteInt64(52, int64(t.Stable11))
	e.WriteString(53, t.Notset15)
	e.WriteString(54, t.Notset16)
	e.WriteString(55, t.Notset17)
	e.WriteInt64(56, int64(t.Stable12))
	return e.Bytes()
}

// EncodeTokenEncrypt 编码TokenEncrypt消息
func EncodeTokenEncrypt(t *TokenEncrypt) []byte {
	e := NewProtobufEncoder()
	if t.One != nil {
		e.WriteMessage(1, EncodeTokenEncryptOne(t.One))
	}
	e.WriteString(2, t.LastToken)
	e.WriteString(3, t.OS)
	e.WriteString(4, t.SdkVer)
	e.WriteInt64(5, int64(t.SdkVerCode))
	e.WriteString(6, t.MsAppID)
	e.WriteString(7, t.AppVersion)
	e.WriteString(8, t.DeviceID)
	if t.Two != nil {
		e.WriteMessage(9, EncodeTokenEncryptTwo(t.Two))
	}
	e.WriteInt64(11, int64(t.Stable1))
	e.WriteString(12, t.Unknown2)
	e.WriteString(15, t.Notset1)
	e.WriteInt64(16, int64(t.Stable2))
	return e.Bytes()
}

// MakeTokenEncrypt 构造与 Python 版本一致的 TokenEncrypt 消息
func MakeTokenEncrypt(stime int64, deviceID string) *TokenEncrypt {
	const (
		stableValue        = 1999997
		stableLargeValue   = 118396899328 << 1
		unknown4Value      = 15887769600
		unknown5Value      = 141133357056
		sdkVerCodeValue    = 84017184 << 1
		defaultSdkVersion  = "v05.02.02-alpha.12-ov-android"
		defaultAppVersion  = "40.6.3"
		defaultMsAppID     = "1233"
		defaultBuildID     = "BP1A.250505.005"
		defaultAppLanguage = "en_"
		defaultTimeZone    = "America/New_York,-5"
		defaultOSVersion   = "15"
		defaultBrand       = "google"
		defaultModel       = "Pixel 6"
	)

	stable := uint64(stableValue)
	stableLarge := uint64(stableLargeValue)
	unknown4 := uint64(unknown4Value)
	unknown5 := uint64(unknown5Value)
	sdkVerCode := uint64(sdkVerCodeValue)

	mediadrmID := GenerateFakeMediadrmID()
	apkPath := GenerateRandomAPKPath()
	uuid := generateRandomUUID()

	unknown2Rand := uint64(rand.Intn(50)+1) << 1
	unknown3Rand := uint64(rand.Intn(51)+50) << 1
	launchTime := uint64((stime - int64(rand.Intn(16)+5)) << 1)
	expiredTime := uint64((stime + 14350) << 1)
	sendTime := uint64(stime << 1)

	r1 := rand.Intn(3) + 1
	r2 := rand.Intn(89) + 111
	r3 := rand.Intn(117) + 50
	defaultGateway := fmt.Sprintf("192.168.%d.%d", r2, r1)
	ipDNS := fmt.Sprintf("192.168.%d.%d", r2, r3)
	ipArray := fmt.Sprintf("[\"%s\",\"0.0.0.0\"]", ipDNS)

	one := &TokenEncrypt_One{
		Notset1:            "!notset!",
		Changshang:         defaultBrand,
		Xinghao:            defaultModel,
		Notset2:            "!notset!",
		OS:                 "Android",
		OSVersion:          defaultOSVersion,
		TokenEncryptOneOne: &TokenEncrypt_One_One{Unknown1: 3472332702763464752},
		DensityDPI:         840,
		BuildID:            defaultBuildID,
		OSBuildTime:        3346620910,
		AppLanguage:        defaultAppLanguage,
		TimeZone:           defaultTimeZone,
		Unknown2:           unknown2Rand,
		Unknown3:           unknown3Rand,
		Unknown4:           unknown4,
		Stable1:            stableLarge,
		Stable2:            stableLarge,
		Unknown5:           unknown5,
		Notset3:            "!notset!",
		Notset4:            "!notset!",
		AndroidID:          randomHexString(8),
		Notset5:            "!notset!",
		Notset6:            "!notset!",
		MediaDrm:           mediadrmID,
		LaunchTime:         launchTime,
		BootID:             generateRandomUUID(),
		Unknown6:           755285745664,
		Notset7:            "!notset!",
		Stable3:            stable,
		Stable4:            stable,
		Notset8:            "!notset!",
		DefaultGateway:     defaultGateway,
		IPDNS:              ipDNS,
		Notset9:            "!notset!",
		Netset10:           "",
		Stable5:            stable,
		Stable6:            stable,
		Notset11:           "!notset!",
		Notset12:           "!notset!",
		IPArray:            ipArray,
		Stable7:            stable,
		Stable8:            stable,
		Notset13:           "!notset!",
		ExpiredTime:        expiredTime,
		SendTime:           sendTime,
		InstallPath:        apkPath,
		OSAPI:              70,
		Stable9:            stable,
		Notset14:           "!notset!",
		Stable10:           stable,
		Stable11:           stable,
		Notset15:           "!notset!",
		Notset16:           "!notset!",
		Notset17:           "!notset!",
		Stable12:           stable,
	}

	two := &TokenEncrypt_Two{
		S6: []uint64{48, 48, 48, 48, 48, 48, 48, 48},
	}

	return &TokenEncrypt{
		One:        one,
		LastToken:  "",
		OS:         "android",
		SdkVer:     defaultSdkVersion,
		SdkVerCode: sdkVerCode,
		MsAppID:    defaultMsAppID,
		AppVersion: defaultAppVersion,
		DeviceID:   deviceID,
		Two:        two,
		Stable1:    stable,
		Unknown2:   uuid,
		Notset1:    "!notset!",
		Stable2:    stable,
	}
}

// MakeTokenEncryptHex 创建TokenEncrypt消息并返回hex字符串 - 完全匹配Python: make_token_encrypt(stime:int, device_id:str)
func MakeTokenEncryptHex(stime int64, deviceID string) (string, error) {
	tokenEncrypt := MakeTokenEncrypt(stime, deviceID)
	tokenEncryptBytes := EncodeTokenEncrypt(tokenEncrypt)
	return hex.EncodeToString(tokenEncryptBytes), nil
}

// MakeTokenRequest 创建TokenRequest消息并序列化 - 完全匹配Python: make_token_request(token_encrypt:hex, utime:int)
func MakeTokenRequest(tokenEncryptHex string, utime int64) (string, error) {
	tokenEncryptBytes, err := hex.DecodeString(tokenEncryptHex)
	if err != nil {
		return "", err
	}

	e := NewProtobufEncoder()
	// Python: token_request.s1 = 538969122<<1
	e.WriteInt64(1, 538969122<<1)
	// Python: token_request.s2 = 2
	e.WriteInt32(2, 2)
	// Python: token_request.s3 = 2
	e.WriteInt32(3, 2)
	// Python: token_request.token_encrypt = bytes.fromhex(token_encrypt)
	e.WriteBytes(4, tokenEncryptBytes)
	// Python: token_request.utime = utime <<1
	e.WriteInt64(5, utime<<1)

	return hex.EncodeToString(e.Bytes()), nil
}

// DecodeTokenDecrypt 解码TokenDecrypt消息
func DecodeTokenDecrypt(data []byte) (*TokenDecrypt, error) {
	d := NewProtobufDecoder(data)
	result := &TokenDecrypt{}

	for d.HasMore() {
		fieldNum, wireType, err := d.ReadTag()
		if err != nil {
			break
		}

		switch fieldNum {
		case 1:
			result.Token, _ = d.ReadString()
		case 2:
			val, _ := d.ReadInt64()
			result.ExpireTime = val
			result.S2 = uint64(val)
		default:
			d.Skip(wireType)
		}
	}

	return result, nil
}

// MakeTokenDecrypt 解析TokenDecrypt消息
func MakeTokenDecrypt(hexData string) (*TokenDecrypt, error) {
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, err
	}
	return DecodeTokenDecrypt(data)
}

// DecodeTokenResponse 解码TokenResponse消息
func DecodeTokenResponse(data []byte) (*TokenResponse, error) {
	d := NewProtobufDecoder(data)
	result := &TokenResponse{}

	for d.HasMore() {
		fieldNum, wireType, err := d.ReadTag()
		if err != nil {
			break
		}

		switch fieldNum {
		case 1:
			v, _ := d.ReadInt64()
			result.S1 = uint64(v)
		case 2:
			v, _ := d.ReadInt64()
			result.S2 = uint64(v)
		case 5:
			v, _ := d.ReadInt64()
			result.S3 = uint64(v)
		case 6:
			innerData, _ := d.ReadBytes()
			result.TokenDecryptBytes = innerData
			result.TokenDecrypt = hex.EncodeToString(innerData)
		default:
			d.Skip(wireType)
		}
	}

	return result, nil
}

// MakeTokenResponse 解析TokenResponse消息
func MakeTokenResponse(hexData string) (*TokenResponse, error) {
	hexData = strings.TrimSpace(hexData)
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, err
	}
	return DecodeTokenResponse(data)
}

// GetTokenInfo 从TokenResponse中获取token信息
func GetTokenInfo(response *TokenResponse) (string, int64) {
	if response != nil && response.TokenDecrypt != "" {
		// 需要解密TokenDecrypt
		tokenDecrypt, err := MakeTokenDecrypt(response.TokenDecrypt)
		if err == nil && tokenDecrypt != nil {
			return tokenDecrypt.Token, tokenDecrypt.ExpireTime
		}
	}
	return "", 0
}
