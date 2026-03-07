package pcap

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

const (
	EthernetOltType layers.EthernetType = 0x88b6
	Timeout         time.Duration       = time.Nanosecond * 400
)

var (
	_ sources.Sources = &Pcap{}

	optsSerealize = gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
)

type Pcap struct {
	macNetDev sources.HardwareAddr

	pcapPackets chan gopacket.Packet
	returnPkts  chan *sources.Packet
	onceStart   *sync.Once

	CorrentRequest uint16
	reqLock        *sync.Mutex
	fnCallbacks    *util.SyncMap[uint16, sources.ASyncFn]

	log          *slog.Logger
	pcapHandle   *pcap.Handle
	packetSource *gopacket.PacketSource
}

func getInterfaceMAC(name string) sources.HardwareAddr {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return sources.HardwareAddr{0, 0, 0, 0, 0, 0}
	}
	return sources.HardwareAddr(iface.HardwareAddr)
}

func New(ifaceName string) (sources.Sources, error) {
	macNetDev := getInterfaceMAC(ifaceName)
	handle, err := pcap.OpenLive(ifaceName, 1600, true, Timeout)
	if err != nil {
		return nil, fmt.Errorf("error on open interface: %v", err)
	}
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	return &Pcap{
		macNetDev: sources.HardwareAddr(macNetDev),
		log:       slog.New(slog.DiscardHandler),

		pcapPackets: packetSource.Packets(),
		returnPkts:  make(chan *sources.Packet, 5000),
		onceStart:   &sync.Once{},

		CorrentRequest: 0,
		reqLock:        &sync.Mutex{},
		fnCallbacks:    util.NewSyncMap[uint16, sources.ASyncFn](),

		pcapHandle:   handle,
		packetSource: packetSource,
	}, nil
}

func (pcap *Pcap) Close() error {
	if pcap.returnPkts != nil {
		select {
		case <-pcap.returnPkts:
		default:
			close(pcap.returnPkts)
			pcap.returnPkts = nil
		}
	}
	if pcap.pcapHandle != nil {
		pcap.pcapHandle.Close()
	}
	return nil
}

func (pcap Pcap) MacAddr() sources.HardwareAddr {
	return pcap.macNetDev
}

func (pcap *Pcap) Slog(log *slog.Logger) {
	pcap.log = slog.New(log.Handler().WithAttrs([]slog.Attr{slog.String("sources", pcap.macNetDev.String())}))
}

func (pcap *Pcap) assignerPkt(raw *sources.Packet) {
	pcap.reqLock.Lock()
	defer pcap.reqLock.Unlock()
	if pcap.CorrentRequest == sources.LimitForU16 {
		pcap.log.Debug("Request ID Overflow")
		pcap.fnCallbacks.CheckClear(func(key uint16, fn sources.ASyncFn) bool {
			fn(&sources.Packet{Error: sources.ErrTimeout})
			return true
		})
	}
	pcap.CorrentRequest++
	pcap.log.Debug("assigner pkt ID", "id", pcap.CorrentRequest)
	raw.RequestID = pcap.CorrentRequest
}

func (pcap *Pcap) sendPkt(pkt *sources.Packet) error {
	if pkt.Mac.String() == (&sources.HardwareAddr{}).String() {
		return sources.ErrNotValid
	}
	pcap.log.Debug("sending pkt", "destination", pkt.Mac, "requestID", pkt.RequestID, "data", pkt.Encode())
	buffer := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buffer, optsSerealize, &layers.Ethernet{
		SrcMAC:       pcap.macNetDev.Net(),
		DstMAC:       pkt.Mac.Net(),
		EthernetType: layers.EthernetType(EthernetOltType),
	}, gopacket.Payload(pkt.Encode()))
	return pcap.pcapHandle.WritePacketData(buffer.Bytes())
}

func (pcap *Pcap) AsyncSend(pkt *sources.Packet, fn sources.ASyncFn) {
	if fn == nil {
		panic("require function")
	}
	pcap.assignerPkt(pkt)
	pcap.fnCallbacks.Set(pkt.RequestID, fn)
	pcap.sendPkt(pkt)
}

func (pcap *Pcap) Send(pkt *sources.Packet, timeout ...time.Duration) (*sources.Packet, error) {
	pcap.assignerPkt(pkt)
	if len(timeout) > 0 {
		timeup := timeout[0]

		back := make(chan *sources.Packet, 1)
		defer func() {
			pcap.fnCallbacks.Del(pkt.RequestID)
			close(back)
			back = nil
		}()

		pcap.fnCallbacks.Set(pkt.RequestID, func(pkt *sources.Packet) bool {
			if back != nil {
				select {
				case back <- pkt:
				default:
				}
			}
			return true
		})
		pcap.sendPkt(pkt)

		select {
		case pkt := <-back:
			return pkt, nil
		case <-time.After(timeup):
		}

		return nil, sources.ErrTimeout
	}

	// Send packet
	return nil, pcap.sendPkt(pkt)
}

func (pcap *Pcap) processResponses(workderID int) {
	defer pcap.Close()
	log := pcap.log.With("WorkderID_PCAP", workderID)

	for pkt := range pcap.pcapPackets {
		ethLayer := pkt.Layer(layers.LayerTypeEthernet)
		if ethLayer == nil {
			continue
		}

		eth := ethLayer.(*layers.Ethernet)
		if eth.SrcMAC.String() == pcap.macNetDev.String() {
			log.Debug("Ignoring own packets", "Source MAC Addr", eth.SrcMAC.String(), "Destination MAC Addr", eth.DstMAC.String(), "Ethernet type", eth.EthernetType.String())
			continue
		}

		if !(eth.EthernetType == layers.EthernetType(EthernetOltType) && sources.IsOltPacket(eth.Payload)) {
			log.Debug("Droping non-olt packet", "Source MAC Addr", eth.SrcMAC.String(), "Destination MAC Addr", eth.DstMAC.String(), "Ethernet type", eth.EthernetType.String(), "Payload", hex.EncodeToString(eth.Payload))
			continue
		}

		pkt := sources.Parse(eth.SrcMAC, eth.Payload)
		if pkt.Error != nil {
			log.Error("Error on parse packet", "error", pkt.Error)
			continue
		} else {
			log.Debug("Packet", "requestID", pkt.RequestID, "pkt", pkt)
		}

		if fn, ok := pcap.fnCallbacks.Get(pkt.RequestID); pkt.Error == nil && ok {
			if fn(pkt) {
				pcap.fnCallbacks.Del(pkt.RequestID)
			}
			continue
		}
		pcap.returnPkts <- pkt
	}
}

func (pcap *Pcap) GetPkts() <-chan *sources.Packet {
	go pcap.onceStart.Do(func() {
		for workderID := range runtime.NumCPU() {
			go pcap.processResponses(workderID)
		}
	})
	return pcap.returnPkts
}
