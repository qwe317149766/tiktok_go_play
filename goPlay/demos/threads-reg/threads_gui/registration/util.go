package registration

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Sleep pauses the current goroutine for at least the duration d.
// It returns immediately if the context is cancelled.
func Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// GeoInfo represents the response from the IP geolocation API
type GeoInfo struct {
	IP       string `json:"ip"`
	Timezone string `json:"timezone"`
}

var geoSem = make(chan struct{}, 50)

// EncryptPassword4NodeCompatible mimicks Node.js encryption
func EncryptPassword4NodeCompatible(pubKeyRaw string, keyId int, password string) (string, string, error) {
	if pubKeyRaw == "" {
		return "", "", fmt.Errorf("public key is empty")
	}

	randKey := make([]byte, 32)
	if _, err := rand.Read(randKey); err != nil {
		return "", "", err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return "", "", err
	}

	var pubKeyBytes []byte
	if block, _ := pem.Decode([]byte(pubKeyRaw)); block != nil {
		pubKeyBytes = block.Bytes
	} else {
		decoded, err := base64.StdEncoding.DecodeString(pubKeyRaw)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode public key: %v", err)
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

	rsaEncrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPubKey, randKey)
	if err != nil {
		return "", "", err
	}

	timeStr := strconv.FormatInt(time.Now().Unix(), 10)
	block, err := aes.NewCipher(randKey)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 12)
	if err != nil {
		return "", "", err
	}

	sealed := gcm.Seal(nil, iv, []byte(password), []byte(timeStr))
	aesEncrypted := sealed[:len(sealed)-16]
	authTag := sealed[len(sealed)-16:]

	buf := bytes.NewBuffer(nil)
	buf.WriteByte(1)
	buf.WriteByte(byte(keyId))
	buf.Write(iv)
	binary.Write(buf, binary.LittleEndian, uint16(len(rsaEncrypted)))
	buf.Write(rsaEncrypted)
	buf.Write(authTag)
	buf.Write(aesEncrypted)

	result := "#PWD_INSTAGRAM:4:" + base64.StdEncoding.EncodeToString(buf.Bytes())
	return result, timeStr, nil
}

// GenerateRandomBirthday generates random day, month, year
func GenerateRandomBirthday(minAge, maxAge int) (int, int, int) {
	now := time.Now()
	startYear := now.Year() - maxAge
	endYear := now.Year() - minAge
	r := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	year := r.Intn(endYear-startYear+1) + startYear
	month := r.Intn(12) + 1
	day := r.Intn(28) + 1
	return day, month, year
}

func GetTimestampByBirthdayAndTZ(dateStr string, tz string) (int64, error) {
	layout := "02-01-2006"
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return 0, err
	}
	t, err := time.ParseInLocation(layout, dateStr, loc)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// GetIPAndTimezone fetches IP and Timezone
func GetIPAndTimezone(ctx context.Context, proxyURL string, logger func(string)) (*GeoInfo, error) {
	endpoints := []string{
		"http://ip-api.com/json/",
		"https://ipapi.co/json/",
		"https://ipinfo.io/json",
		"https://freeipapi.com/api/json",
	}

	transport := &http.Transport{DisableKeepAlives: true}
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second, Transport: transport}

	for i := 0; i < 3; i++ { // limit retries
		for _, targetUrl := range endpoints {
			req, _ := http.NewRequestWithContext(ctx, "GET", targetUrl, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

			geoSem <- struct{}{}
			resp, err := client.Do(req)
			<-geoSem

			if err != nil {
				if logger != nil {
					logger(fmt.Sprintf("Geo fetch failed %s: %v", targetUrl, err))
				}
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var rawMap map[string]interface{}
			if err := json.Unmarshal(body, &rawMap); err != nil {
				continue
			}

			var info GeoInfo
			if ip, ok := rawMap["query"].(string); ok {
				info.IP = ip
			} else if ip, ok := rawMap["ip"].(string); ok {
				info.IP = ip
			} else if ip, ok := rawMap["ipAddress"].(string); ok {
				info.IP = ip
			}

			if tz, ok := rawMap["timezone"].(string); ok {
				info.Timezone = tz
			} else if tz, ok := rawMap["timeZone"].(string); ok {
				info.Timezone = tz
			}

			if info.IP != "" && info.Timezone != "" {
				return &info, nil
			}
		}
		if err := Sleep(ctx, 2*time.Second); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("failed to get geo info")
}

// GenerateRandomPassword generates a random password with specified length
func GenerateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@~"
	var seededRand *mrand.Rand = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// GenerateRandomAndroidID generates a random 16-character hex string
func GenerateRandomAndroidID() string {
	const charset = "0123456789abcdef"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[mrand.Intn(len(charset))]
	}
	return fmt.Sprintf("%s-%s", "android", string(b))
}
