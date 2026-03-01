package sources

type PacketRaw struct {
	Error error
	Mac   HardwareAddr
	Pkt   *Packet
}

// Generic interface to Send and Recive OLT Packets
type Sources interface {
	Close() error                        // Stop process packets
	MacAddr() HardwareAddr               // Hardware mac address if exist
	GetPkts() <-chan *PacketRaw    // Get data
	SendPkt(pkt *PacketRaw) error // Send data packet
}
