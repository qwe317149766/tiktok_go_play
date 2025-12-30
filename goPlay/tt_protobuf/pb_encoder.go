package tt_protobuf

import (
	"encoding/binary"
	"encoding/hex"
	"io"
)

// ProtobufEncoder protobuf编码器
type ProtobufEncoder struct {
	buf []byte
}

// NewProtobufEncoder 创建新的编码器
func NewProtobufEncoder() *ProtobufEncoder {
	return &ProtobufEncoder{buf: make([]byte, 0, 256)}
}

// Bytes 返回编码后的字节
func (e *ProtobufEncoder) Bytes() []byte {
	return e.buf
}

// Hex 返回编码后的16进制字符串
func (e *ProtobufEncoder) Hex() string {
	return hex.EncodeToString(e.buf)
}

// WriteVarint 写入varint
func (e *ProtobufEncoder) WriteVarint(v uint64) {
	for v >= 0x80 {
		e.buf = append(e.buf, byte(v)|0x80)
		v >>= 7
	}
	e.buf = append(e.buf, byte(v))
}

// WriteSignedVarint 写入有符号varint (zigzag编码)
func (e *ProtobufEncoder) WriteSignedVarint(v int64) {
	uv := uint64((v << 1) ^ (v >> 63))
	e.WriteVarint(uv)
}

// WriteTag 写入字段标签
func (e *ProtobufEncoder) WriteTag(fieldNum int, wireType int) {
	e.WriteVarint(uint64((fieldNum << 3) | wireType))
}

// WriteInt32 写入int32字段
func (e *ProtobufEncoder) WriteInt32(fieldNum int, v int32) {
	if v == 0 {
		return
	}
	e.WriteTag(fieldNum, 0)
	e.WriteVarint(uint64(v))
}

// WriteInt64 写入int64字段
func (e *ProtobufEncoder) WriteInt64(fieldNum int, v int64) {
	if v == 0 {
		return
	}
	e.WriteTag(fieldNum, 0)
	e.WriteVarint(uint64(v))
}

// WriteString 写入string字段
func (e *ProtobufEncoder) WriteString(fieldNum int, v string) {
	if v == "" {
		return
	}
	e.WriteTag(fieldNum, 2)
	e.WriteVarint(uint64(len(v)))
	e.buf = append(e.buf, []byte(v)...)
}

// WriteBytes 写入bytes字段
func (e *ProtobufEncoder) WriteBytes(fieldNum int, v []byte) {
	if len(v) == 0 {
		return
	}
	e.WriteTag(fieldNum, 2)
	e.WriteVarint(uint64(len(v)))
	e.buf = append(e.buf, v...)
}

// WriteFixed64 写入固定64位字段
func (e *ProtobufEncoder) WriteFixed64(fieldNum int, v uint64) {
	if v == 0 {
		return
	}
	e.WriteTag(fieldNum, 1)
	tmp := make([]byte, 8)
	binary.LittleEndian.PutUint64(tmp, v)
	e.buf = append(e.buf, tmp...)
}

// WriteMessage 写入嵌套消息字段
func (e *ProtobufEncoder) WriteMessage(fieldNum int, v []byte) {
	if len(v) == 0 {
		return
	}
	e.WriteTag(fieldNum, 2)
	e.WriteVarint(uint64(len(v)))
	e.buf = append(e.buf, v...)
}

// ProtobufDecoder protobuf解码器
type ProtobufDecoder struct {
	buf []byte
	pos int
}

// NewProtobufDecoder 创建新的解码器
func NewProtobufDecoder(data []byte) *ProtobufDecoder {
	return &ProtobufDecoder{buf: data, pos: 0}
}

// ReadVarint 读取varint
func (d *ProtobufDecoder) ReadVarint() (uint64, error) {
	var v uint64
	var shift uint
	for {
		if d.pos >= len(d.buf) {
			return 0, io.EOF
		}
		b := d.buf[d.pos]
		d.pos++
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			break
		}
		shift += 7
	}
	return v, nil
}

// ReadTag 读取字段标签
func (d *ProtobufDecoder) ReadTag() (int, int, error) {
	v, err := d.ReadVarint()
	if err != nil {
		return 0, 0, err
	}
	return int(v >> 3), int(v & 7), nil
}

// ReadInt32 读取int32
func (d *ProtobufDecoder) ReadInt32() (int32, error) {
	v, err := d.ReadVarint()
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}

// ReadInt64 读取int64
func (d *ProtobufDecoder) ReadInt64() (int64, error) {
	v, err := d.ReadVarint()
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

// ReadString 读取string
func (d *ProtobufDecoder) ReadString() (string, error) {
	length, err := d.ReadVarint()
	if err != nil {
		return "", err
	}
	if d.pos+int(length) > len(d.buf) {
		return "", io.EOF
	}
	s := string(d.buf[d.pos : d.pos+int(length)])
	d.pos += int(length)
	return s, nil
}

// ReadBytes 读取bytes
func (d *ProtobufDecoder) ReadBytes() ([]byte, error) {
	length, err := d.ReadVarint()
	if err != nil {
		return nil, err
	}
	if d.pos+int(length) > len(d.buf) {
		return nil, io.EOF
	}
	b := make([]byte, length)
	copy(b, d.buf[d.pos:d.pos+int(length)])
	d.pos += int(length)
	return b, nil
}

// Skip 跳过字段
func (d *ProtobufDecoder) Skip(wireType int) error {
	switch wireType {
	case 0: // varint
		_, err := d.ReadVarint()
		return err
	case 1: // 64-bit
		d.pos += 8
	case 2: // length-delimited
		length, err := d.ReadVarint()
		if err != nil {
			return err
		}
		d.pos += int(length)
	case 5: // 32-bit
		d.pos += 4
	}
	return nil
}

// HasMore 是否还有更多数据
func (d *ProtobufDecoder) HasMore() bool {
	return d.pos < len(d.buf)
}

// EncodeFixed32 编码固定32位数字
func EncodeFixed32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// DecodeFixed32 解码固定32位数字
func DecodeFixed32(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b)
}
