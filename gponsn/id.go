package gponsn

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrSnNull    = errors.New("null sn id")
	ErrSnInvalid = errors.New("sn is invalid data")
)

type Sn [8]byte

func (sn Sn) ID() uint32 {
	return binary.BigEndian.Uint32(sn[4:])
}

func (sn Sn) IDHex() string {
	return strings.ToUpper(hex.EncodeToString(sn[4:]))
}

func (sn Sn) Vendor() Vendor {
	return Vendor(binary.BigEndian.Uint32(sn[:4]))
}

func (sn Sn) String() string {
	return fmt.Sprintf("%s-%s", sn.Vendor().HexString(), sn.IDHex())
}

func (sn Sn) MarshalText() ([]byte, error) {
	return []byte(sn.Vendor().HexString() + sn.IDHex()), nil
}

func (sn *Sn) UnmarshalText(data []byte) error {
	if len(data) < 8 {
		return ErrSnInvalid
	}

	if data[4] == '-' {
		data = bytes.Join([][]byte{data[:4], data[5:]}, []byte{})
	}
	return sn.UnmarshalBinary(data)
}

func (sn Sn) MarshalBinary() ([]byte, error) {
	if sn.ID() == 0 {
		return nil, ErrSnNull
	}
	return sn[:], nil
}

// Decode this type of SN:
//
//   - [4byte vendor][4byte id]
//   - [4byte vendor][8byte id]
//   - [8byte vendor][8byte id]
func (sn *Sn) UnmarshalBinary(data []byte) (err error) {
	var vendor, id [4]byte
	var n int

	switch len(data) {
	case 8:
		copy(vendor[:], data[:4])
		copy(id[:], data[4:])
	case 12:
		copy(vendor[:], data[:4])
		n, err = hex.Decode(id[:], data[4:])
		if n != 4 {
			return ErrSnInvalid
		}
	case 16:
		hex.Decode(vendor[:], data[:8])
		hex.Decode(id[:], data[8:])
	default:
		return ErrSnInvalid
	}

	vendorSn := Vendor(binary.BigEndian.Uint32(vendor[:]))
	idSn := binary.BigEndian.Uint32(id[:])

	if idSn == 0 || !vendorSn.IsValid() {
		return ErrSnNull
	}

	copy(sn[:4], vendor[:])
	copy(sn[4:], id[:])
	return nil
}

func Parse(data []byte) (Sn, error) {
	var sn Sn
	if err := sn.UnmarshalText(data); err != nil {
		return Sn{}, err
	}
	return sn, nil
}
