package sources

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

var (
	_ encoding.BinaryMarshaler   = Packet{}
	_ encoding.BinaryUnmarshaler = &Packet{}

	_ encoding.TextMarshaler   = HardwareAddr{}
	_ encoding.TextUnmarshaler = &HardwareAddr{}

	ErrNotValid = errors.New("packet not valid")
	ErrNoMagic  = errors.New("packet have magic in star section")

	staticBuff = make([]byte, 50)

	NullMac  = HardwareAddr(make([]byte, 6))
	OltMagic = []byte{0xb9, 0x58, 0xd6, 0x3a}
)

type HardwareAddr net.HardwareAddr

func (hd HardwareAddr) Net() net.HardwareAddr { return net.HardwareAddr(hd) }

func (hd HardwareAddr) String() string { return net.HardwareAddr(hd).String() }

func (hd HardwareAddr) MarshalText() ([]byte, error) {
	return []byte(hd.String()), nil
}

func (hd *HardwareAddr) UnmarshalText(text []byte) error {
	mac, err := net.ParseMAC(string(text))
	if err != nil {
		return err
	}
	*hd = HardwareAddr(mac)
	return nil
}

type Packet struct {
	RequestID   uint16 `json:"request_id"`
	RequestType uint16 `json:"request_type"`
	Flag0       uint8  `json:"flag0"`
	Flag1       uint8  `json:"flag1"`
	Flag2       uint8  `json:"flag2"`
	Flag3       uint8  `json:"flag3"` // Old `Status` flag

	Error error        `json:"error"`
	Mac   HardwareAddr `json:"mac"`
	Data  []byte       `json:"data"`
}

func IsOltPacket(raw []byte) bool {
	return bytes.HasPrefix(raw, OltMagic)
}

func Parse[K HardwareAddr | net.HardwareAddr](mac K, data []byte) *Packet {
	newMac := HardwareAddr(mac)
	switch newMac.String() {
	case "", (&HardwareAddr{}).String(), NullMac.String():
		return &Packet{Error: ErrNotValid}
	default:
		if len(newMac) < 6 {
			return &Packet{Error: ErrNotValid}
		}
	}
	pkt := &Packet{Mac: newMac}
	if err := pkt.UnmarshalBinary(data); err != nil {
		pkt.Error = err
	}
	return pkt
}

func (pkt *Packet) UnmarshalBinary(raw []byte) error {
	if !IsOltPacket(raw) {
		return ErrNotValid
	}

	if raw, ok := bytes.CutPrefix(raw, OltMagic); ok {
		header, raw := raw[:8], raw[8:]
		pkt.Data = raw

		pkt.RequestID = binary.BigEndian.Uint16(header[:2])
		pkt.RequestType = binary.BigEndian.Uint16(header[2:4])
		pkt.Flag3 = uint8(header[4])
		pkt.Flag0 = uint8(header[5])
		pkt.Flag1 = uint8(header[6])
		pkt.Flag2 = uint8(header[7])
		return nil
	}

	return ErrNoMagic
}

func (pkt Packet) Encode() []byte {
	buff := new(bytes.Buffer)
	buff.Grow(50)

	buff.Write(OltMagic)
	binary.Write(buff, binary.BigEndian, pkt.RequestID)
	binary.Write(buff, binary.BigEndian, pkt.RequestType)
	buff.WriteByte(pkt.Flag3)
	buff.WriteByte(pkt.Flag0)
	buff.WriteByte(pkt.Flag1)
	buff.WriteByte(pkt.Flag2)
	if pkt.Data != nil {
		buff.Write(pkt.Data)
	}
	if buff.Len() < 50 {
		buff.Write(staticBuff[:50-buff.Len()])
	}
	return buff.Bytes()
}

func (pkt Packet) MarshalBinary() ([]byte, error) { return pkt.Encode(), nil }
func (pkt Packet) String() string {
	return fmt.Sprintf("%02x-%02x-%02x-%02x", pkt.Flag0, pkt.Flag1, pkt.Flag2, pkt.Flag3)
}

// Return new Packet with Broadcast MAC Address
func New() *Packet {
	return &Packet{
		Mac: bytes.Clone(BroadcastMAC),
	}
}

func (pkt Packet) Clone() *Packet {
	return &Packet{
		RequestID:   pkt.RequestID,
		RequestType: pkt.RequestType,
		Flag0:       pkt.Flag0,
		Flag1:       pkt.Flag1,
		Flag2:       pkt.Flag2,
		Flag3:       pkt.Flag3,

		Error: pkt.Error,
		Mac:   bytes.Clone(pkt.Mac),
		Data:  bytes.Clone(pkt.Data),
	}
}

func (pkt *Packet) SetMacAddr(mac HardwareAddr) *Packet {
	pkt = pkt.Clone()
	pkt.Mac = bytes.Clone(mac)
	return pkt
}

func (pkt *Packet) SetNetMacAddr(mac net.HardwareAddr) *Packet {
	pkt = pkt.Clone()
	pkt.Mac = HardwareAddr(mac)
	return pkt
}

func (pkt *Packet) SetRequestID(data uint16) *Packet {
	pkt = pkt.Clone()
	pkt.RequestID = data
	return pkt
}

func (pkt *Packet) SetRequestType(data uint16) *Packet {
	pkt = pkt.Clone()
	pkt.RequestType = data
	return pkt
}

func (pkt *Packet) SetFlag0(data uint8) *Packet {
	pkt = pkt.Clone()
	pkt.Flag0 = data
	return pkt
}

func (pkt *Packet) SetFlag1(data uint8) *Packet {
	pkt = pkt.Clone()
	pkt.Flag1 = data
	return pkt
}

func (pkt *Packet) SetFlag2(data uint8) *Packet {
	pkt = pkt.Clone()
	pkt.Flag2 = data
	return pkt
}

func (pkt *Packet) SetFlag3(data uint8) *Packet {
	pkt = pkt.Clone()
	pkt.Flag3 = data
	return pkt
}
