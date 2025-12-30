package tt_protobuf

import (
	"encoding/hex"
)

// MakeSeedEncrypt builds a SeedEncrypt message.
func MakeSeedEncrypt(sessionID, deviceID, os, sdkVersion string) (string, error) {
	if os == "" {
		os = "android"
	}
	if sdkVersion == "" {
		sdkVersion = "v05.02.00"
	}

	seedEncrypt := &SeedEncrypt{
		Session:    sessionID,
		DeviceID:   deviceID,
		OS:         os,
		SDKVersion: sdkVersion,
	}

	seedEncryptBytes := EncodeSeedEncrypt(seedEncrypt)
	return hex.EncodeToString(seedEncryptBytes), nil
}

// EncodeSeedEncrypt encodes SeedEncrypt using the protobuf wire format.
func EncodeSeedEncrypt(s *SeedEncrypt) []byte {
	e := NewProtobufEncoder()
	e.WriteString(1, s.Session)
	e.WriteString(2, s.DeviceID)
	e.WriteString(3, s.OS)
	e.WriteString(4, s.SDKVersion)
	return e.Bytes()
}

// MakeSeedRequest creates a SeedRequest message and serializes it.
func MakeSeedRequest(seedEncryptHex string, utime int64) (string, error) {
	seedEncryptBytes, err := hex.DecodeString(seedEncryptHex)
	if err != nil {
		return "", err
	}

	e := NewProtobufEncoder()
	e.WriteInt64(1, 538969122<<1)
	e.WriteInt32(2, 2)
	e.WriteInt32(3, 4)
	e.WriteBytes(4, seedEncryptBytes)
	e.WriteInt64(5, utime<<1)

	return hex.EncodeToString(e.Bytes()), nil
}

// DecodeSeedDecrypt decodes the SeedDecrypt payload.
func DecodeSeedDecrypt(data []byte) (*SeedDecrypt, error) {
	d := NewProtobufDecoder(data)
	result := &SeedDecrypt{ExtraInfo: &SeedInfo{}}

	for d.HasMore() {
		fieldNum, wireType, err := d.ReadTag()
		if err != nil {
			break
		}

		switch fieldNum {
		case 1:
			result.Seed, _ = d.ReadString()
		case 2:
			innerData, _ := d.ReadBytes()
			DecodeSeedInfo(innerData, result.ExtraInfo)
		default:
			d.Skip(wireType)
		}
	}

	return result, nil
}

// DecodeSeedInfo decodes the SeedInfo message.
func DecodeSeedInfo(data []byte, result *SeedInfo) {
	d := NewProtobufDecoder(data)
	for d.HasMore() {
		fieldNum, wireType, err := d.ReadTag()
		if err != nil {
			break
		}
		switch fieldNum {
		case 1:
			result.Algorithm, _ = d.ReadString()
		default:
			d.Skip(wireType)
		}
	}
}

// MakeSeedDecrypt parses a hex encoded SeedDecrypt body.
func MakeSeedDecrypt(decryptHex string) (*SeedDecrypt, error) {
	data, err := hex.DecodeString(decryptHex)
	if err != nil {
		return nil, err
	}
	return DecodeSeedDecrypt(data)
}

// DecodeSeedResponse decodes a SeedResponse message.
func DecodeSeedResponse(data []byte) (*SeedResponse, error) {
	d := NewProtobufDecoder(data)
	result := &SeedResponse{}

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
			result.SeedDecryptBytes = innerData
			result.SeedDecrypt = hex.EncodeToString(innerData)
		default:
			d.Skip(wireType)
		}
	}

	return result, nil
}

// MakeSeedResponse parses a hex encoded SeedResponse message.
func MakeSeedResponse(responseHex string) (*SeedResponse, error) {
	data, err := hex.DecodeString(responseHex)
	if err != nil {
		return nil, err
	}
	return DecodeSeedResponse(data)
}
