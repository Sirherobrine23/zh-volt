package zhvolt

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/gponsn"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

const MaxONU = 128

type ONUStatus uint8

const (
	ONUStatusOffline = ONUStatus(iota)
	ONUStatusOnline
	_
	ONUStatusDisconnected
	_
	_
	_
	ONUStatusOMCI
)

func (status ONUStatus) MarshalText() ([]byte, error) {
	switch status {
	case ONUStatusOffline:
		return []byte("Offline"), nil
	case ONUStatusOnline:
		return []byte("Online"), nil
	case ONUStatusDisconnected:
		return []byte("Disconnected"), nil
	case ONUStatusOMCI:
		return []byte("OMCI"), nil
	}
	return fmt.Appendf(nil, "Unknown (%d)", uint8(status)), nil
}

type ONU struct {
	ID      uint8     `json:"id"`
	Status  ONUStatus `json:"status"`
	Time    []byte    `json:"last_update"`
	SN      gponsn.Sn `json:"gpon_sn"`
	TxPower float64   `json:"tx_power"`
	RxPower float64   `json:"rx_power"`
	Voltage uint64    `json:"voltage"`
	Current uint64    `json:"current"`
	Temp    float32   `json:"temperature"`
}

type Olt struct {
	Up              time.Time            `json:"up"`
	Mac             sources.HardwareAddr `json:"mac_addr"`
	FirmwareVersion string               `json:"fw_ver"`
	DNA             string               `json:"dna"`
	Temperature     float64              `json:"temperature"`
	MaxTemperature  float64              `json:"max_temperature"`
	OMCIMode        int                  `json:"omci_mode"`
	OMCIErr         int                  `json:"omci_err"`
	OnlineONU       uint8                `json:"online_onu"`
	MaxONU          uint8                `json:"max_onu"`
	ONUs            []*ONU               `json:"onu"`

	Log         *log.Logger
	parent      *OltManeger                               // Olt parent to send packets
	oltCallback *util.SyncMap[uint16, oltManegerCallback] // once callbacks
}

func NewOlt(parent *OltManeger, macAddr sources.HardwareAddr) *Olt {
	return &Olt{
		parent:      parent,
		Mac:         macAddr,
		ONUs:        make([]*ONU, 0),
		Log:         log.New(parent.Log.Writer(), fmt.Sprintf("OLT %s: ", macAddr), defaultLogFlag),
		oltCallback: util.NewSyncMap[uint16, oltManegerCallback](),
	}
}

func (olt *Olt) sendPacketCallback(raw *sources.PacketRaw, call oltManegerCallback) error {
	if olt.parent.assignerRequestID(raw) {
		olt.oltCallback.Clear()
		// olt.parent.oltCallback
	}
	olt.oltCallback.Set(raw.Pkt.RequestID, call)
	return olt.parent.pktSource.SendPkt(raw)
}

func (olt *Olt) sendPacketWait(raw *sources.PacketRaw, timeout time.Duration) (*sources.PacketRaw, error) {
	back := make(chan *sources.PacketRaw, 1)
	defer close(back)

	err := olt.sendPacketCallback(raw, func(pkt *sources.PacketRaw, olt *Olt, remove func()) {
		defer remove()
		if back != nil {
			select {
			case <-back:
			default:
				back <- pkt
			}
		}
	})

	if err != nil {
		return nil, err
	}

	select {
	case pkt := <-back:
		return pkt, nil
	case <-time.After(timeout):
		return nil, ErrCallbackTimeout
	}
}

func (olt *Olt) OltInfo() {
	for range time.Tick(time.Second) {
		pkt, err := olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x03, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get current temperature: %s", err)
			continue
		}
		olt.Temperature = (float64(binary.BigEndian.Uint16(pkt.Pkt.Data)) / 100) * 2

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x04, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get max temperature: %s", err)
			continue
		}
		olt.MaxTemperature = (float64(binary.BigEndian.Uint16(pkt.Pkt.Data)) / 100) * 2

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag2: 0xff}}, time.Second)
		if err != nil {
			olt.Log.Printf("error on get current ONUs online: %s", err)
			continue
		}
		olt.OnlineONU = pkt.Pkt.Data[0]
	}
}

func (olt *Olt) processONU(onuID uint8, onu *ONU) {
	for {
		olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x2, Flag1: 0x03, Flag2: onu.ID}}, time.Second)
		olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x2, Flag1: 0x0d, Flag2: onu.ID}}, time.Second)
		pkt, err := olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag1: 0x06, Flag2: onu.ID}}, time.Second)
		if err != nil {
			olt.Log.Printf("ONU %d return error on get GPON SN: %s", onuID, err)
			continue
		}

		onu.SN, err = gponsn.Parse(pkt.Pkt.Data[:8])
		switch err {
		case nil:
		case gponsn.ErrSnNull:
			continue
		case gponsn.ErrSnInvalid, gponsn.ErrVendorInvalid:
			olt.Log.Printf("error on decode GPON SN for %d", onuID)
			continue
		default:
			olt.Log.Printf("error on decode %d: %s", onuID, err)
		}

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag1: 0x01, Flag2: onu.ID}}, time.Second)
		if err != nil {
			olt.Log.Printf("ONU %d return error on get status: %s", onuID, err)
			continue
		}
		onu.Status = ONUStatus(pkt.Pkt.Data[0])
		if onu.Status > ONUStatusOnline {
			continue
		}

		pkt, err = olt.sendPacketWait(&sources.PacketRaw{Mac: olt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag1: 0x07, Flag2: onu.ID}}, time.Second)
		if err != nil {
			olt.Log.Printf("ONU %d return error on get pkt: %s", onuID, err)
			continue
		}
		onu.Time = pkt.Pkt.Data[:6]

		<-time.After(time.Second*2)
	}
}

func (olt *Olt) OnuUpdate() {
	olt.ONUs = make([]*ONU, olt.MaxONU)
	for onuID := range olt.MaxONU {
		onu := new(ONU)
		onu.ID = onuID
		olt.ONUs[onuID] = onu
		go olt.processONU(onuID, onu)
	}
}

func (olt *Olt) Packet(pkt *sources.PacketRaw) {
	if fn, ok := olt.oltCallback.Get(pkt.Pkt.RequestID); ok {
		olt.oltCallback.Del(pkt.Pkt.RequestID)
		fn(pkt, olt, func() {})
		return
	}

	olt.Log.Printf("PacketID %d droped process", pkt.Pkt.RequestID)
}
