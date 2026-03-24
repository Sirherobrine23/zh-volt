use crate::olt::olt::Olt;
use crate::olt::packets::Packet;
use crate::olt::pcap::{self, ErrPcap, Pcap};
use std::{
	collections::HashMap,
	sync::{
		Arc, Mutex, RwLock,
		atomic::{AtomicBool, Ordering},
	},
	thread,
	time::Duration,
};

pub type SharedOltState = Arc<RwLock<HashMap<String, Arc<Mutex<Olt>>>>>;
pub type PcapShare = Arc<Mutex<pcap::Pcap>>;

pub fn new_share() -> SharedOltState {
	Arc::new(RwLock::new(HashMap::new()))
}

pub fn get_olts_vec(olts: SharedOltState) -> Result<Vec<Olt>, ErrPcap> {
	let mut data: Vec<Olt> = vec![];
	for (_, p) in olts.read().unwrap().iter() {
		{
			let olt = p.try_lock().map_err(|_| ErrPcap::LockError)?;
			data.push(olt.clone());
		}
	}

	Ok(data)
}

pub fn new_pcap_dev(net_dev: String) -> Result<PcapShare, ErrPcap> {
	let dev = match Pcap::new(net_dev) {
		Err(err) => return Err(err),
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
		let pkts_channel = {
			let pcap_guard = self.pcap.lock().unwrap();
			pcap_guard.pkts.clone()
		};

		{
			// let running = self.running;
			let olts = self.olts.clone();
			let pcap = self.pcap.clone();
			thread::spawn(move || {
				loop {
					println!("Sending ping broadcast to detect new OLTs");
					let olts = olts.clone();
					let pcap = pcap.clone();
					let pcap_callback = pcap.clone();
					{
						let _ = pcap_callback.lock().unwrap().send_packat_callback(
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
										thread::spawn(move || Olt::process(arc_olt_thread, mac_dst, pcap_clone));
									}
								};
								Ok(true)
							},
						);
					}
					thread::sleep(Duration::from_secs(80));
				}
			});
		}

		while self.running.load(Ordering::Relaxed) {
			println!("waiting drop pkt...");
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
