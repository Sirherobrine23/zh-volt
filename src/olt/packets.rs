use std::fmt;

use byteorder::{BigEndian, ByteOrder};
use hex;
use macaddr;
use serde::{Deserialize, Serialize};

use crate::olt::{olt_maneger::PcapShare, pcap::ErrPcap};

pub const OLT_MAGIC: [u8; 4] = [0xb9, 0x58, 0xd6, 0x3a];
pub const BROADCAST_MAC: macaddr::MacAddr6 = macaddr::MacAddr6::broadcast();
pub const HEADER_SIZE: usize = 4 + 2 + 2 + 1 + 1 + 1 + 1;

#[derive(Debug)]
pub enum ErrPacket {
	ErrNotMagic,
	Hex(hex::FromHexError),
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Packet {
	pub request_id: u16,
	pub request_type: u16,
	pub flag0: u8,
	pub flag1: u8,
	pub flag2: u8,
	pub flag3: u8,

	pub data: Vec<u8>,
	pub mac_dst: macaddr::MacAddr6,

	#[serde(skip_serializing)]
	#[serde(skip_deserializing)]
	pcap: Option<PcapShare>,
}

impl Packet {
	pub fn new() -> Self {
		return Packet {
			request_id: 0,
			request_type: 0,
			flag0: 0,
			flag1: 0,
			flag2: 0,
			flag3: 0,
			data: vec![],
			mac_dst: BROADCAST_MAC.clone(),
			pcap: None,
		};
	}

	pub fn from_bytes(data: &[u8], mac_dst: macaddr::MacAddr6) -> Result<Self, ErrPacket> {
		if data.len() < 8 || data[..4] != OLT_MAGIC {
			return Err(ErrPacket::ErrNotMagic);
		}
		let data = &data[4..];
		let request_id = BigEndian::read_u16(&data[0..2]);
		let request_type = BigEndian::read_u16(&data[2..4]);
		let flag3 = data[4];
		let flag0 = data[5];
		let flag1 = data[6];
		let flag2 = data[7];
		let data = &data[8..];
		Ok(Packet {
			request_id,
			request_type,
			flag0,
			flag1,
			flag2,
			flag3,
			data: data.to_vec(),
			mac_dst,
			pcap: None,
		})
	}

	pub fn to_bytes(&self) -> Vec<u8> {
		let mut buff: Vec<u8> = Vec::from(OLT_MAGIC.clone());
		buff.extend_from_slice(&self.request_id.to_be_bytes());
		buff.extend_from_slice(&self.request_type.to_be_bytes());
		buff.push(self.flag3);
		buff.push(self.flag0);
		buff.push(self.flag1);
		buff.push(self.flag2);
		buff.extend_from_slice(&self.data);
		if buff.len() < 50 {
			buff.resize(50, 0);
		}
		buff
	}

	pub fn set_mac(&self, mac: macaddr::MacAddr6) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.mac_dst = mac;
		new_pkt
	}
	pub fn set_request_id(&self, request_id: u16) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.request_id = request_id;
		new_pkt
	}
	pub fn set_request_type(&self, request_type: u16) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.request_type = request_type;
		new_pkt
	}
	pub fn set_flag0(&self, flag0: u8) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.flag0 = flag0;
		new_pkt
	}
	pub fn set_flag1(&self, flag1: u8) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.flag1 = flag1;
		new_pkt
	}
	pub fn set_flag2(&self, flag2: u8) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.flag2 = flag2;
		new_pkt
	}
	pub fn set_flag3(&self, flag3: u8) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.flag3 = flag3;
		new_pkt
	}
	pub fn set_data(&self, data: &[u8]) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.data = data.to_vec();
		new_pkt
	}
	pub fn set_data_vec(&self, data: Vec<u8>) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.data = data;
		new_pkt
	}

	pub fn set_pcap(&self, pcap: PcapShare) -> Self {
		let mut new_pkt = self.clone();
		new_pkt.pcap = Some(pcap);
		new_pkt
	}
	pub fn send(&self) -> Option<ErrPcap> {
		if let Some(pcap) = &self.pcap {
			match pcap.lock().unwrap().send_packet(self) {
				Ok(_) => return None,
				Err(err) => return Some(err),
			}
		}
		ErrPcap::ErrNoDevice.into()
	}
	pub fn send_recv(&self, timeout: std::time::Duration) -> Result<Packet, ErrPcap> {
		if let Some(pcap) = &self.pcap {
			return pcap.lock().unwrap().send_recv(timeout, self);
		}
		Err(ErrPcap::ErrNoDevice)
	}
}

impl fmt::Display for Packet {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		write!(
			f,
			"Packet {{ request_id: {}, request_type: {}, flag0: {}, flag1: {}, flag2: {}, flag3: {}, data: {} bytes, mac_dst: {} }}",
			self.request_id,
			self.request_type,
			self.flag0,
			self.flag1,
			self.flag2,
			self.flag3,
			self.data.len(),
			self.mac_dst
		)
	}
}
