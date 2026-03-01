package zhvolt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

const (
	defaultLogFlag = log.Ldate | log.Ltime | log.LUTC | log.Lmsgprefix
	u16Limit       = ^uint16(0)
)

var (
	BroadcastMAC = sources.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	ErrCallbackTimeout = errors.New("timeout on callback call")
)

type oltManegerCallback func(pkt *sources.PacketRaw, olt *Olt, remove func())

type OltManeger struct {
	CorrentRequest uint16
	Log            *log.Logger
	Verbose        uint8

	context   context.Context
	pktSource sources.Sources
	newDev    chan struct{}

	oltCallback map[uint16]oltManegerCallback
	olt         *util.SyncMap[string, *Olt]
	onceStart   *sync.Once
}

func NewOltProcess(ctx context.Context, source sources.Sources, logWrite io.Writer) (*OltManeger, error) {
	oltManeger := &OltManeger{
		context:        ctx,
		pktSource:      source,
		Log:            log.New(logWrite, fmt.Sprintf("OLT Maneger %s: ", source.MacAddr()), defaultLogFlag),
		Verbose:        1,
		newDev:         make(chan struct{}, 1),
		CorrentRequest: 0,
		olt:            util.NewSyncMap[string, *Olt](),
		onceStart:      &sync.Once{},
		oltCallback:    map[uint16]oltManegerCallback{},
	}

	return oltManeger, nil
}

func (man OltManeger) Olts() map[string]*Olt {
	return man.olt.Clone()
}

func (maneger *OltManeger) Close() (err error) {
	if maneger.pktSource != nil {
		if err = maneger.pktSource.Close(); err != nil {
			return
		}
	}
	if maneger.newDev != nil {
		select {
		case <-maneger.newDev:
		default:
			close(maneger.newDev)
			maneger.newDev = nil
		}
	}
	return
}

// Wait for fist device locate
func (man *OltManeger) Wait(timeout time.Duration) bool {
	if man.newDev == nil {
		return true
	}

	select {
	case <-man.newDev:
		return true
	case <-time.After(timeout):
		man.Close()
		return false
	}
}

func (man *OltManeger) assignerRequestID(raw *sources.PacketRaw) (overflow bool) {
	if u16Limit == man.CorrentRequest {
		overflow = true
	}
	man.CorrentRequest++
	raw.Pkt.RequestID = man.CorrentRequest
	return
}

func (man *OltManeger) sendPacket(raw *sources.PacketRaw) error {
	if man.assignerRequestID(raw) {
		for id := range man.oltCallback {
			delete(man.oltCallback, id)
		}
	}
	return man.pktSource.SendPkt(raw)
}

func (man *OltManeger) sendPacketCallback(raw *sources.PacketRaw, call oltManegerCallback) error {
	if man.assignerRequestID(raw) {
		for id := range man.oltCallback {
			delete(man.oltCallback, id)
		}
	}
	man.oltCallback[raw.Pkt.RequestID] = call
	return man.pktSource.SendPkt(raw)
}

func (man *OltManeger) sendPacketWait(raw *sources.PacketRaw, timeout time.Duration) (*sources.PacketRaw, error) {
	back := make(chan *sources.PacketRaw, 1)
	defer close(back)

	err := man.sendPacketCallback(raw, func(pkt *sources.PacketRaw, olt *Olt, remove func()) {
		defer remove()
		back <- pkt
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

func (man *OltManeger) processPacket(workerID int) {
	defer man.Close()
	log := log.New(man.Log.Writer(), fmt.Sprintf("worker %d, %s", workerID+1, man.Log.Prefix()), man.Log.Flags())
	for pkt := range man.pktSource.GetPkts() {
		if man.Verbose > 3 {
			if pkt.Error != nil {
				man.Log.Printf("Worker %d: Pkt %s, Error %s", workerID, pkt.Mac, pkt.Error)
			} else {
				man.Log.Printf("Worker %d: Pkt %s, requestID %d", workerID, pkt.Mac, pkt.Pkt.RequestID)
			}
		}

		if pkt.Error != nil {
			log.Printf("Error on get packet from %s: %s", pkt.Mac, pkt.Error)
			continue
		}

		olt, exist := man.olt.Get(pkt.Mac.String())
		if !exist {
			log.Printf("New olt, Mac address %s", pkt.Mac)
			olt = NewOlt(man, pkt.Mac)
			man.olt.Set(pkt.Mac.String(), olt)
			if man.newDev != nil {
				man.newDev <- struct{}{}
				close(man.newDev)
				man.newDev = nil
			}
		}

		if fn, ok := man.oltCallback[pkt.Pkt.RequestID]; ok {
			fn(pkt, olt, func() {
				delete(man.oltCallback, pkt.Pkt.RequestID)
			})
			continue
		}

		olt.Packet(pkt)
	}
}

// Process packets in background
func (man *OltManeger) Start() {
	man.onceStart.Do(func() {
		cores := runtime.NumCPU() * 2
		man.Log.Printf("Starting process packet with %d workers", cores)
		for i := range cores {
			go man.processPacket(i)
		}

		man.sendPacketCallback(&sources.PacketRaw{
			Mac: BroadcastMAC,
			Pkt: &sources.Packet{
				RequestType: 0x000c,
				Flag2:       0xff,
			},
		}, func(pkt *sources.PacketRaw, olt *Olt, _ func()) {
			var err error
			olt.FirmwareVersion = strings.TrimFunc(string(pkt.Pkt.Data), func(r rune) bool {
				return unicode.IsSpace(r) || r == 0x0
			})

			// Get max ONUs
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x18, Flag2: 0xff}}, time.Second)
			if err != nil {
				olt.Log.Printf("error on get max ONUs: %s", err)
				return
			}
			olt.MaxONU = pkt.Pkt.Data[0]
			olt.Log.Printf("Max ONUs: %d", olt.MaxONU)

			// Get OLT DNA
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x08, Flag2: 0xff}}, time.Second)
			if err != nil {
				olt.Log.Printf("error on get OLT DNA: %s", err)
				return
			}
			olt.DNA = hex.EncodeToString(pkt.Pkt.Data[:bytes.IndexByte(pkt.Pkt.Data, 0x0)])
			olt.Log.Printf("OLT DNA: %s", olt.DNA)

			// Get uptime
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x01, Flag2: 0xff}}, time.Second)
			if err != nil {
				olt.Log.Printf("error on get OLT uptime: %s", err)
				return
			}
			now := time.Duration(binary.BigEndian.Uint64(pkt.Pkt.Data)) * 16
			olt.Up = time.UnixMicro(time.Now().UnixMicro()).Add(-now)
			olt.Log.Printf("OLT Up time %s (%s)", olt.Up.Format(time.RFC3339), now)

			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x05, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x06, Flag2: 0xff}})

			// Get current ONUs connected
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag2: 0xff}}, time.Second)
			if err != nil {
				olt.Log.Printf("error on get ONUs connected: %s", err)
				return
			}
			olt.OnlineONU = uint8(pkt.Pkt.Data[0])
			olt.Log.Printf("Current ONU online: %d", olt.OnlineONU)

			// Get current OMCI Mode
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x19, Flag2: 0xff}}, time.Second)
			if err != nil {
				olt.Log.Printf("error on get OMCI Mode: %s", err)
				return
			}
			olt.OMCIMode = int(byte(pkt.Pkt.Data[0]))
			olt.Log.Printf("OMCI Mode %d", olt.OMCIMode)

			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x02, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x07, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0b, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0c, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0d, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x09, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x05, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x07, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x0a, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x06, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x08, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x11, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x12, Flag2: 0xff}})
			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x01, Flag1: 0x13, Flag2: 0xff}})

			// ??
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag0: 0x02, Flag1: 0x01, Flag2: 0x00}}, time.Second)
			if err != nil {
				olt.Log.Printf("??: %s", err)
				return
			}

			// OLT Config
			pkt, err = man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000f, Flag1: 0x09, Flag2: 0xff}}, time.Second)
			if err != nil {
				olt.Log.Printf("OLT Config error: %s", err)
				return
			}
			olt.Log.Printf("OLT Config %s", hex.EncodeToString(pkt.Pkt.Data))

			man.sendPacket(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x10, Flag2: 0xff}})
			man.sendPacketWait(&sources.PacketRaw{Mac: pkt.Mac, Pkt: &sources.Packet{RequestType: 0x000c, Flag1: 0x03, Flag2: 0xff}}, time.Second)

			go olt.OltInfo()
			olt.OnuUpdate()
		})
	})
}

// Send: Type: 0x0014, Status 0x02, Flag0: 0x01, Flag1: 0x0d, Flag2: 0xff, Data = data
// func (man *OltManeger) SetAuthLoid(data string) error {
	
// }

// Send: Type: 0x0014, Status 0x02, Flag0: 0x01, Flag1: 0x0c, Flag2: 0xff, Data = data
// func (man *OltManeger) SetAuthPass(data string) error {
	
// }