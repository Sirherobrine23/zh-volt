use std::collections::HashMap;
use std::pin::Pin;
use std::sync::atomic::{AtomicU16, Ordering};
use std::sync::mpsc::{RecvTimeoutError, Sender};
use std::sync::{Arc, RwLock};
use std::sync::{Mutex, mpsc, mpsc::Receiver};

use crate::olt::packets;
use pnet::{
	datalink::{self, Channel::Ethernet, DataLinkSender, NetworkInterface},
	packet::{
		Packet as PnetPacket,
		ethernet::{EtherType, EthernetPacket, MutableEthernetPacket},
	},
};

pub const OLT_ETHERNET_TYPE: EtherType = EtherType(0x88b6);

#[derive(Debug)]
pub enum ErrPcap {
	ErrNoDevice,
	ErrCannotSend,
	ErrCannotRecive,
	LockError,
	Timeout,
	Packet(packets::ErrPacket),
	Io(std::io::Error),
}

pub type Callback = Box<dyn Fn(packets::Packet) -> Result<bool, ErrPcap> + Send + Sync + 'static>;
pub type AsyncCallback = Box<
	dyn Fn(
			packets::Packet,
		) -> Pin<Box<dyn Future<Output = Result<bool, ErrPcap>> + Send + Sync + 'static>>
		+ Send
		+ Sync
		+ 'static,
>;

pub struct Pcap {
	pub mac_net_dev: macaddr::MacAddr,
	pub dev_iface: NetworkInterface,
	request_id: Arc<AtomicU16>,
	request_processs: Arc<RwLock<HashMap<u16, Callback>>>,
	async_responders: Arc<RwLock<HashMap<u16, Sender<packets::Packet>>>>,
	pub pkts: Arc<Mutex<Receiver<packets::Packet>>>,
	pub tx: Box<dyn DataLinkSender>,
}

impl std::fmt::Debug for Pcap {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.debug_struct("Pcap")
			.field("mac_net_dev", &self.mac_net_dev)
			.field("dev_iface", &self.dev_iface)
			.finish()
	}
}

impl Pcap {
	pub fn new(dev_name: String) -> Result<Self, ErrPcap> {
		let dev = match datalink::interfaces()
			.into_iter()
			.find(|iface| iface.name == dev_name)
		{
			None => return Err(ErrPcap::ErrNoDevice),
			Some(iface) => iface,
		};

		let dev_clone = dev.clone();
		let mac_octets = dev.mac.ok_or(ErrPcap::ErrNoDevice)?.octets();
		let mac: macaddr::MacAddr = mac_octets.into();

		let (tx, mut rx) = match datalink::channel(&dev_clone, Default::default()) {
			Ok(Ethernet(tx, rx)) => (tx, rx),
			Err(e) => return Err(ErrPcap::Io(e)),
			Ok(_) => return Err(ErrPcap::ErrNoDevice),
		};

		let (pkt_tx, pkt_rx) = mpsc::channel::<packets::Packet>();

		let dev_clone = dev.clone();
		let pcpa = Pcap {
			mac_net_dev: mac,
			dev_iface: dev.clone(),
			request_id: Arc::new(AtomicU16::new(1)),
			request_processs: Arc::new(RwLock::new(HashMap::new())),
			async_responders: Arc::new(RwLock::new(HashMap::new())),
			pkts: Arc::new(Mutex::new(pkt_rx)),
			tx,
		};

		let response = pcpa.async_responders.clone();
		let request_processs = pcpa.request_processs.clone();
		std::thread::spawn(move || {
			loop {
				let packet_data = match rx.next() {
					Err(_) => break,
					Ok(r) => r,
				};

				let pkt = EthernetPacket::new(packet_data);
				if pkt.is_none() {
					continue;
				}

				let pkt = pkt.unwrap();
				if pkt.get_ethertype() != OLT_ETHERNET_TYPE || pkt.get_source() == dev_clone.mac.unwrap() {
					continue;
				}
				let mac = pkt.get_source().octets();

				match packets::Packet::from_bytes(
					pkt.payload(),
					macaddr::MacAddr6::new(mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]),
				) {
					Err(err) => {
						println!("Error parsing packet: {:?}", err);
						continue;
					}
					Ok(p) => {
						let id = p.request_id;

						if let Ok(mut responders) = response.write() {
							if let Some(callback) = responders.remove(&p.request_id) {
								let _ = callback.send(p);
								continue;
							}
						}

						if let Ok(mut request_processors) = request_processs.write() {
							if let Some(callback) = request_processors.remove(&p.request_id) {
								match callback(p) {
									Err(_) => continue,
									Ok(true) => continue,
									Ok(false) => {
										request_processors.insert(id, callback);
										continue;
									}
								}
							}
						}

						{
							let _ = pkt_tx.send(p);
						}
					}
				};
			}
		});

		Ok(pcpa)
	}

	fn get_id(&mut self) -> u16 {
		self.request_id.fetch_add(1, Ordering::Relaxed)
	}

	fn send(&mut self, pkt: &packets::Packet) -> Result<(), ErrPcap> {
		let pkt_size = 64 + pkt.data.len();
		let mut buffer = vec![0u8; pkt_size];

		let mut packet = MutableEthernetPacket::new(&mut buffer[..]).unwrap();
		packet.set_source(self.dev_iface.mac.unwrap());

		let mac = pkt.mac_dst.into_array();
		packet.set_destination(datalink::MacAddr(
			mac[0], mac[1], mac[2], mac[3], mac[4], mac[5],
		));

		packet.set_ethertype(OLT_ETHERNET_TYPE);
		packet.set_payload(&pkt.to_bytes());

		match self.tx.send_to(packet.packet(), None) {
			Some(Ok(())) => Ok(()),
			Some(Err(err)) => Err(ErrPcap::Io(err)),
			None => Err(ErrPcap::ErrCannotSend),
		}
	}

	// Send packet and capture in future by olt struct
	pub fn send_packet(&mut self, pkt: &packets::Packet) -> Result<(), ErrPcap> {
		let mut pkt = pkt.clone();
		pkt.request_id = self.get_id();
		self.send(&pkt)
	}

	pub fn send_packat_callback<F>(
		&mut self,
		pkt: &packets::Packet,
		callback: F,
	) -> Result<(), ErrPcap>
	where
		F: Fn(packets::Packet) -> Result<bool, ErrPcap> + Send + Sync + 'static,
	{
		let mut pkt = pkt.clone();
		pkt.request_id = self.get_id();
		match self.request_processs.write() {
			Err(_) => return Err(ErrPcap::LockError),
			Ok(mut rw) => rw.insert(pkt.request_id, Box::new(callback)),
		};
		self.send(&pkt)
	}

	pub fn recv_packet(&mut self) -> Result<packets::Packet, ErrPcap> {
		match self.pkts.lock().unwrap().recv() {
			Err(_) => Err(ErrPcap::ErrCannotRecive),
			Ok(pkt) => Ok(pkt),
		}
	}

	// Send and wait to recive packet response
	pub fn send_recv(
		&mut self,
		timeout: std::time::Duration,
		pkt: &packets::Packet,
	) -> Result<packets::Packet, ErrPcap> {
		let mut pkt = pkt.clone();
		let id = self.get_id();
		pkt.request_id = id;

		let (tx, rx) = mpsc::channel();
		{
			let mut responders = self
				.async_responders
				.write()
				.map_err(|_| ErrPcap::LockError)?;
			responders.insert(id, tx);
		}

		self.send(&pkt)?;
		match rx.recv_timeout(timeout) {
			Err(RecvTimeoutError::Timeout) => Err(ErrPcap::Timeout),
			Err(RecvTimeoutError::Disconnected) => Err(ErrPcap::ErrCannotRecive),
			Ok(pkt) => Ok(pkt),
		}
	}
}
