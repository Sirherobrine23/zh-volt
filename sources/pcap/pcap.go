package pcap

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
)

const (
	EthernetOltType = 0x88b6
	Timeout         = time.Microsecond * 5
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
	returnPkts  chan *sources.PacketRaw
	onceStart   *sync.Once

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

		pcapPackets: packetSource.Packets(),
		returnPkts:  make(chan *sources.PacketRaw, 5000),
		onceStart:   &sync.Once{},

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

func (pcap *Pcap) GetPkts() <-chan *sources.PacketRaw {
	go pcap.onceStart.Do(func() {
		defer pcap.Close()
		for pkt := range pcap.pcapPackets {
			defer pcap.Close()
			ethLayer := pkt.Layer(layers.LayerTypeEthernet)
			if ethLayer == nil {
				continue
			}

			eth := ethLayer.(*layers.Ethernet)
			if eth.SrcMAC.String() == pcap.macNetDev.String() {
				continue
			}

			if eth.EthernetType != layers.EthernetType(EthernetOltType) || !sources.IsOltPacket(eth.Payload) {
				continue
			}

			if sources.IsOltPacket(eth.Payload) {
				pkt, err := sources.Parse(eth.Payload)
				if pcap.returnPkts == nil {
					return
				}
				pcap.returnPkts <- &sources.PacketRaw{
					Error: err,
					Pkt:   pkt,
					Mac:   sources.HardwareAddr(eth.SrcMAC),
				}
			}
		}
	})

	return pcap.returnPkts
}

func (pcap *Pcap) SendPkt(pkt *sources.PacketRaw) error {
	data, err := pkt.Pkt.MarshalBinary()
	if err != nil {
		return err
	}

	eth := &layers.Ethernet{
		SrcMAC:       pcap.macNetDev.Net(),
		DstMAC:       pkt.Mac.Net(),
		EthernetType: layers.EthernetType(EthernetOltType),
	}

	buffer := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buffer, optsSerealize, eth, gopacket.Payload(data))
	return pcap.pcapHandle.WritePacketData(buffer.Bytes())
}
