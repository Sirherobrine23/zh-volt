use crate::olt::olt::Olt;
use crate::olt::packets::Packet;
use crate::olt::pcap::{self, ErrPcap, Pcap};
use std::collections::HashMap;
use std::sync::{
	Arc, Mutex, RwLock,
	atomic::{AtomicBool, Ordering},
};

pub type SharedOltState = Arc<RwLock<HashMap<String, Arc<Mutex<Olt>>>>>;
pub type PcapShare = Arc<Mutex<pcap::Pcap>>;

pub fn new_share() -> SharedOltState {
	Arc::new(RwLock::new(HashMap::new()))
}

pub fn new_pcap_dev(net_dev: String) -> Result<PcapShare, ErrPcap> {
	let dev = match Pcap::new(net_dev) {
		Err(err) => panic!("Error starting Pcap: {:?}", err),
		Ok(dev) => Arc::new(Mutex::new(dev)),
	};
	Ok(dev)
}

pub struct OltManager {
	pub pcap: PcapShare,
	pub running: AtomicBool,
	pub olts: SharedOltState,
}

impl OltManager {
	pub fn new(pcap: PcapShare, olts: SharedOltState) -> Self {
		OltManager {
			running: AtomicBool::new(true),
			pcap,
			olts,
		}
	}

	pub fn run(&mut self) {
		println!("Send ping to OLTs with broadcast");
		{
			match self
				.pcap
				.lock()
				.unwrap()
				.send_packet(&Packet::new().set_request_type(0x000c).set_flag2(0xff))
			{
				Ok(_) => println!("Ping broadcast sent!"),
				Err(err) => panic!("Error sending ping packet to get OLTs: {:?}", err),
			}
		}

		let olts = self.olts.clone();

		{
			let pcap = self.pcap.clone();
			let _ = self.pcap.lock().unwrap().send_packat_callback(
				&Packet::new().set_request_type(0x000c).set_flag2(0xff),
				move |pkt: Packet| {
					let olts = olts.clone();
					let pcap = pcap.clone();
					let pkt_clone = pkt.clone();
					let mut olts_guard = { olts.write().unwrap() };

					match olts_guard.get_mut(&pkt.mac_dst.to_string()) {
						Some(olt) => olt.lock().unwrap().process_packet(&pkt_clone),
						None => {
							let mac_dst = pkt.mac_dst;
							let arc_olt = Arc::new(Mutex::new(Olt::new(mac_dst, pkt)));
							olts_guard.insert(mac_dst.to_string(), arc_olt.clone());
							println!("New OLT Discovery: {}", mac_dst.to_string());
							let pcap_clone = pcap.clone();
							let arc_olt_thread = arc_olt.clone();
							std::thread::spawn(move || Olt::process(arc_olt_thread, mac_dst, pcap_clone));
						}
					};
					Ok(true)
				},
			);
			println!("sedd_packat_async_callback");
		}

		let pkts_channel = {
			let pcap_guard = self.pcap.lock().unwrap();
			pcap_guard.pkts.clone()
		};

		while self.running.load(Ordering::Relaxed) {
			println!("waiting pkt...");

			let pkt = match pkts_channel.lock().unwrap().recv() {
				Err(_) => {
					self.running.store(false, Ordering::Relaxed);
					break;
				}
				Ok(pkt) => pkt,
			};

			let dst = pkt.mac_dst.to_string();
			let mut olts = self.olts.write().unwrap();
			if let Some(olt) = olts.get_mut(&dst) {
				olt.lock().unwrap().process_packet(&pkt);
			}
		}
	}
}
