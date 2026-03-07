package olt

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/gponsn"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
)

const MaxONU uint8 = 128

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

func (status ONUStatus) String() string {
	switch status {
	case ONUStatusOffline:
		return "Offline"
	case ONUStatusOnline:
		return "Online"
	case ONUStatusDisconnected:
		return "Disconnected"
	case ONUStatusOMCI:
		return "OMCI"
	}
	return fmt.Sprintf("Unknown (%d)", uint8(status))
}

func (status ONUStatus) MarshalText() ([]byte, error) {
	return []byte(status.String()), nil
}

type ONU struct {
	ID          uint8         `json:"id"`
	Status      ONUStatus     `json:"status"`
	Uptime      time.Duration `json:"uptime"`
	SN          gponsn.Sn     `json:"gpon_sn"`
	Voltage     uint64        `json:"voltage"`
	Current     uint64        `json:"current"`
	TxPower     float64       `json:"tx_power"`
	RxPower     float64       `json:"rx_power"`
	Temperature float32       `json:"temperature"`
	SetStatus   uint8         `json:"set_status"`

	Request map[int]any `json:"unmaped_requests"`

	Log *slog.Logger `json:"-"`
}

type Olt struct {
	Uptime             time.Duration        `json:"uptime"`
	Mac                sources.HardwareAddr `json:"mac_addr"`
	FirmwareVersion    string               `json:"fw_ver"`
	DNA                string               `json:"dna"`
	Temperature        float64              `json:"temperature"`
	MaxTemperature     float64              `json:"max_temperature"`
	OMCIMode           int                  `json:"omci_mode"`
	OMCIErr            int                  `json:"omci_err"`
	OnlineONU          uint8                `json:"online_onu"`
	MaxONU             uint8                `json:"max_onu"`
	ONUs               []*ONU               `json:"onu"`
	Response           time.Duration        `json:"first_response"`
	TimeoutForResponse time.Duration        `json:"timeout_for_response"`

	Log       *slog.Logger
	parent    *OltManeger // Olt parent to send packets
	onceStart func()
}

func NewOlt(parent *OltManeger, macAddr sources.HardwareAddr, responseTime time.Duration) *Olt {
	var olt *Olt
	olt = &Olt{
		parent: parent,
		Mac:    macAddr,
		ONUs:   make([]*ONU, 0),
		Log:    slog.New(parent.Log.Handler().WithAttrs([]slog.Attr{slog.String("olt", macAddr.String())})),
		onceStart: sync.OnceFunc(func() {
			// startAgin:
			pkt := sources.New().SetMacAddr(macAddr).SetRequestType(0x000c).SetFlag2(0xff)
			// Get max ONUs
			res, err := olt.parent.pktSource.Send(pkt.SetFlag0(0x1).SetFlag1(0x18), olt.TimeoutForResponse)
			if err != nil {
				olt.Log.Error("error on get max ONUs", "error", err)
				return
			}
			olt.MaxONU = min(res.Data[0], MaxONU)
			olt.Log.Info("Max ONUs", "max_onu", olt.MaxONU)

			// Get OLT DNA
			res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x08), olt.TimeoutForResponse)
			if err != nil {
				olt.Log.Error("error on get OLT DNA", "error", err)
				return
			}
			olt.DNA = hex.EncodeToString(bytes.TrimRightFunc(res.Data, unicode.IsControl))
			olt.Log.Info("OLT DNA", "dna", olt.DNA)

			olt.parent.pktSource.Send(pkt.SetFlag0(0x01))
			olt.parent.pktSource.Send(pkt.SetFlag1(0x05))
			olt.parent.pktSource.Send(pkt.SetFlag1(0x06))

			// Get current ONUs connected
			res, err = olt.parent.pktSource.Send(pkt.SetFlag0(0x02), olt.TimeoutForResponse)
			if err != nil {
				olt.Log.Error("error on get ONUs connected", "error", err)
				return
			}
			olt.OnlineONU = uint8(res.Data[0])
			olt.Log.Info("Current ONU online", "online_onu", olt.OnlineONU)

			// Get current OMCI Mode
			res, err = olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x19), olt.TimeoutForResponse)
			if err != nil {
				olt.Log.Error("error on get OMCI Mode", "error", err)
				return
			}
			olt.OMCIMode = int(byte(res.Data[0]))
			olt.Log.Info("OMCI Mode", "mode", olt.OMCIMode)

			olt.parent.pktSource.Send(pkt.SetFlag1(0x02))
			olt.parent.pktSource.Send(pkt.SetFlag1(0x07))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x0b))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x0c))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x0d))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x09))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x05))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x07))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x0a))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x06))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x08))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x11))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x12))
			olt.parent.pktSource.Send(pkt.SetFlag0(0x01).SetFlag1(0x13))

			// ??
			res, err = olt.parent.pktSource.Send(pkt.SetFlag0(0x02).SetFlag1(0x01).SetFlag2(0x00), olt.TimeoutForResponse)
			if err != nil {
				olt.Log.Error("??", "error", err)
				return
			}

			// OLT Config
			res, err = olt.parent.pktSource.Send(&sources.Packet{Mac: res.Mac, RequestType: 0x000f, Flag1: 0x09, Flag2: 0xff}, time.Second)
			if err != nil {
				olt.Log.Error("OLT Config error", "error", err)
				return
			}
			olt.Log.Info("OLT Config", "config", hex.EncodeToString(res.Data))

			olt.parent.pktSource.Send(pkt.SetFlag1(0x10))
			olt.parent.pktSource.Send(pkt.SetFlag1(0x03))

			go olt.OltInfo()
			go olt.fetchONUInfo()
		}),
	}
	olt.UpdateTimeout(responseTime)
	return olt
}

func (olt *Olt) UpdateTimeout(responseTime time.Duration) {
	mult := time.Duration(2)
	if responseTime > time.Second {
		mult = 23
	} else if responseTime > time.Millisecond {
		mult = 15
	} else if responseTime > time.Microsecond {
		mult = 10
	}
	olt.Response, olt.TimeoutForResponse = responseTime, max(responseTime*mult, time.Millisecond*10)
}

func (olt *Olt) SetAuthLoid(loid string) error {
	info, err := olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac,
		RequestType: 0x0014,
		Flag3:       0x02,
		Flag0:       0x01,
		Flag1:       0x0d,
		Flag2:       0xff,
		Data:        []byte(loid),
	}, time.Second*2)
	if err == nil && info.Flag3 != 0x1 {
		err = io.ErrNoProgress
	}
	return err
}

func (olt *Olt) SetAuthPass(pass string) error {
	info, err := olt.parent.pktSource.Send(&sources.Packet{Mac: olt.Mac,
		RequestType: 0x0014,
		Flag3:       0x02,
		Flag0:       0x01,
		Flag1:       0x0c,
		Flag2:       0xff,
		Data:        []byte(pass),
	}, time.Second*2)
	if err == nil && info.Flag3 != 0x1 {
		err = io.ErrNoProgress
	}
	return err
}

func (olt *Olt) OltInfo() {
	pkt := sources.New().SetMacAddr(olt.Mac).SetRequestType(0x000c).SetFlag2(0xff)
	for {
		init := time.Now()
		res, err := olt.parent.pktSource.Send(pkt.SetFlag1(0x03), olt.TimeoutForResponse)
		olt.UpdateTimeout(time.Since(init))
		if err != nil {
			olt.Log.Error("error on get current temperature", "error", err)
			continue
		}
		olt.Temperature = (float64(binary.BigEndian.Uint16(res.Data)) / 100) * 2

		res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x04), olt.TimeoutForResponse)
		if err != nil {
			olt.Log.Error("error on get max temperature", "error", err)
			continue
		}
		olt.MaxTemperature = (float64(binary.BigEndian.Uint16(res.Data)) / 100) * 2

		res, err = olt.parent.pktSource.Send(pkt.SetFlag0(0x02), olt.TimeoutForResponse)
		if err != nil {
			olt.Log.Error("error on get current ONUs online", "error", err)
			continue
		}
		olt.OnlineONU = res.Data[0]

		res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x01), olt.TimeoutForResponse)
		if err != nil {
			olt.Log.Error("error on get OLT uptime", "error", err)
			continue
		}
		olt.Uptime = max(0, ((time.Duration(binary.BigEndian.Uint64(res.Data)) * 16) - (time.Second * 2)).Round(time.Second))

		// Wait 5s to update info
		<-time.After(time.Second)
	}
}

func (olt *Olt) fetchONUInfo() {
	olt.ONUs = make([]*ONU, olt.MaxONU)
	for onuID := range olt.MaxONU {
		onu := new(ONU)
		onu.ID = onuID
		onu.Request = make(map[int]any)
		onu.Log = slog.New(olt.Log.Handler().WithAttrs([]slog.Attr{slog.Int("onu", int(onuID)+1)}))
		olt.ONUs[onuID] = onu
	}

	pkt := &sources.Packet{Mac: olt.Mac, RequestType: 0x000c, Flag0: 0x02, Flag1: 0x01}
	for {
		for onuID := range olt.MaxONU {
			onu, pkt := olt.ONUs[onuID], pkt.SetFlag2(onuID)
			if res, err := olt.parent.pktSource.Send(pkt, time.Second); err == nil {
				prevStatus := onu.Status
				onu.Status = ONUStatus(res.Data[0])
				if prevStatus != onu.Status {
					onu.Log.Info("Changes status", "current", onu.Status, "previus", prevStatus)
				}
				if onu.Status == 0 {
					onu.SN = gponsn.Sn{}
					onu.Uptime = 0
					continue
				} else if err = onu.GetInfo(olt); err != nil {
					onu.Log.Log(olt.parent.context, slog.LevelError, "Error on process Info", "error", err)
				}
			}
		}
		<-time.After(time.Millisecond * 300)
	}
}

func (onu *ONU) GetInfo(olt *Olt) (err error) {
	pkt := sources.New().SetMacAddr(olt.Mac).SetRequestType(0x000c).SetFlag0(0x02).SetFlag2(onu.ID)
	res, err := olt.parent.pktSource.Send(pkt.SetFlag1(0x06), olt.TimeoutForResponse)
	if err != nil {
		return
	} else if onu.SN, err = gponsn.Parse(res.Data[:8]); err != nil {
		return
	}

	res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x10), olt.TimeoutForResponse)
	if err != nil {
		return
	}
	onu.Request[0x10] = int8(res.Data[0])

	// ONU Connection time
	if res, err = olt.parent.pktSource.Send(pkt.SetFlag1(0x02), olt.TimeoutForResponse); err == nil {
		onu.Uptime = max(0, (olt.Uptime - time.Duration(binary.BigEndian.Uint64(res.Data)*16)).Round(time.Second))
	}

	return
}

func (olt *Olt) Packet(pkt *sources.Packet) {
	if olt.FirmwareVersion == "" && pkt.Flag0 == 0 && pkt.Flag1 == 0 && pkt.Flag2 == 0xff && pkt.Flag3 == 1 {
		olt.FirmwareVersion = strings.TrimFunc(string(pkt.Data), unicode.IsControl)
		olt.Log.Debug("olt firmware version", "version", olt.FirmwareVersion)
	}
	olt.onceStart()
	if olt.FirmwareVersion != "" {
		olt.Log.Debug("droped pkt", "requestID", pkt.RequestID, "data", pkt.Data)
	}
}
