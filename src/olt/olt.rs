use byteorder::{BigEndian, ByteOrder};
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use std::sync::{Arc, Mutex};
use std::thread::sleep;
use std::time::Duration;

use crate::olt::olt_maneger::PcapShare;
use crate::olt::packets::{self, Packet};
use crate::sn::gpon_sn::{self, Sn};

pub type MutexONUs = Arc<Mutex<Vec<Onu>>>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ONUStatus {
	Offline,
	Online,
	Disconnected,
	Omci,
	Other(u8),
}

impl From<u8> for ONUStatus {
	fn from(value: u8) -> Self {
		match value {
			0x0 => ONUStatus::Offline,
			0x1 => ONUStatus::Online,
			0x3 => ONUStatus::Disconnected,
			0x7 => ONUStatus::Omci,
			val => ONUStatus::Other(val),
		}
	}
}

impl From<ONUStatus> for u8 {
	fn from(status: ONUStatus) -> u8 {
		match status {
			ONUStatus::Offline => 0x0,
			ONUStatus::Online => 0x1,
			ONUStatus::Disconnected => 0x3,
			ONUStatus::Omci => 0x7,
			ONUStatus::Other(val) => val,
		}
	}
}

impl Serialize for ONUStatus {
	fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
	where
		S: Serializer,
	{
		match self {
			ONUStatus::Offline => serializer.serialize_str("offline"),
			ONUStatus::Online => serializer.serialize_str("online"),
			ONUStatus::Disconnected => serializer.serialize_str("disconnected"),
			ONUStatus::Omci => serializer.serialize_str("omci"),
			ONUStatus::Other(val) => serializer.serialize_str(&format!("unknown(0x{:02x})", val)),
		}
	}
}

impl<'de> Deserialize<'de> for ONUStatus {
	fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
	where
		D: Deserializer<'de>,
	{
		let s = String::deserialize(deserializer)?;
		match s.to_lowercase().as_str() {
			"offline" => Ok(ONUStatus::Offline),
			"online" => Ok(ONUStatus::Online),
			"disconnected" => Ok(ONUStatus::Disconnected),
			"omci" => Ok(ONUStatus::Omci),
			_ => Ok(ONUStatus::Other(0xFF)),
		}
	}
}

fn serialize_mac<S>(mac: &macaddr::MacAddr6, serializer: S) -> Result<S::Ok, S::Error>
where
	S: Serializer,
{
	serializer.serialize_str(&mac.to_string())
}

fn serialize_duration<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
where
	S: Serializer,
{
	// 0s
	// 0m0s
	// 0h0m0s
	// 0d0h0m0s
	let total_secs = duration.as_secs();
	let days = total_secs / 86400;
	let hours = (total_secs % 86400) / 3600;
	let minutes = (total_secs % 3600) / 60;
	let seconds = total_secs % 60;

	serializer.serialize_str(&match (days, hours, minutes, seconds) {
		(d, h, m, s) if d > 0 => format!("{}d{}h{}m{}s", d, h, m, s),
		(0, h, m, s) if h > 0 => format!("{}h{}m{}s", h, m, s),
		(0, 0, m, s) if m > 0 => format!("{}m{}s", m, s),
		_ => format!("{}s", seconds),
	})
}

fn serialize_onus<S>(onus: &MutexONUs, serializer: S) -> Result<S::Ok, S::Error>
where
	S: Serializer,
{
	let onus = onus.lock().unwrap();
	onus.serialize(serializer)
}

#[derive(Serialize, Deserialize, Clone)]
pub struct Onu {
	pub id: u8,
	pub status: ONUStatus,
	#[serde(serialize_with = "serialize_duration")]
	pub uptime: Duration,
	pub sn: gpon_sn::Sn,
	pub voltage: u64,
	pub current: u64,
	pub tx_power: u64,
	pub rx_power: u64,
	pub temperature: f64,
}

impl Onu {
	pub fn new(id: u8) -> Self {
		Onu {
			id,
			status: ONUStatus::Offline,
			uptime: Duration::new(0, 0),
			sn: gpon_sn::Sn::default(),
			voltage: 0,
			current: 0,
			tx_power: 0,
			rx_power: 0,
			temperature: 0.0,
		}
	}

	fn set_status(self: &mut Self, status: ONUStatus) {
		self.status = status;
	}

	fn set_uptime(self: &mut Self, uptime: Duration) {
		self.uptime = uptime;
	}

	fn set_sn(self: &mut Self, sn: gpon_sn::Sn) {
		self.sn = sn;
	}

	fn _set_voltage(self: &mut Self, voltage: u64) {
		self.voltage = voltage;
	}

	fn _set_current(self: &mut Self, current: u64) {
		self.current = current;
	}

	fn _set_tx_power(self: &mut Self, tx_power: u64) {
		self.tx_power = tx_power;
	}

	fn _set_rx_power(self: &mut Self, rx_power: u64) {
		self.rx_power = rx_power;
	}

	fn _set_temperature(self: &mut Self, temperature: f64) {
		self.temperature = temperature;
	}
}

#[derive(Serialize, Clone)]
pub struct Olt {
	#[serde(serialize_with = "serialize_duration")]
	pub uptime: Duration,
	#[serde(serialize_with = "serialize_mac")]
	pub mac_addr: macaddr::MacAddr6,
	pub firmware_version: String,
	pub olt_dna: String,
	pub temperature: f64,
	pub max_temperature: f64,
	pub omci_mode: u8,
	pub omci_error: u8,
	pub online_onu: u8,
	pub max_onu: u8,
	#[serde(serialize_with = "serialize_onus")]
	pub onus: MutexONUs,
}

impl Olt {
	pub fn new(mac_addr: macaddr::MacAddr6, pkt: Packet) -> Self {
		Olt {
			uptime: Duration::new(0, 0),
			mac_addr,
			firmware_version: String::from_utf8_lossy(&pkt.data)
				.trim_matches('\0')
				.to_string(),
			olt_dna: String::new(),
			temperature: 0.0,
			max_temperature: 0.0,
			omci_mode: 0,
			omci_error: 0,
			online_onu: 0,
			max_onu: 0,
			onus: Arc::new(Mutex::new(Vec::new())),
		}
	}

	pub fn process(olt_arc: Arc<Mutex<Olt>>, mac_dst: macaddr::MacAddr6, pcap: PcapShare) {
		let pkt = Packet::new()
			.set_pcap(pcap.clone())
			.set_mac(mac_dst.clone())
			.set_request_type(0x000c)
			.set_flag2(0xff);

		// Get max ONUs
		let max_onu = match pkt
			.set_flag0(0x1)
			.set_flag1(0x18)
			.send_recv(Duration::from_secs(10))
		{
			Err(_) => {
				println!("Error sending packet to get OLT max ONUs");
				return;
			}
			Ok(res) => {
				let max_onu: u8 = res.data[0];
				{
					let mut olt = olt_arc.lock().unwrap();
					olt.max_onu = max_onu;

					println!("Max ONU: {:?}", max_onu);
					let mut onus = olt.onus.lock().unwrap();
					for i in 0..max_onu {
						onus.push(Onu::new(i));
					}
				}
				max_onu
			}
		};

		// Get OLT DNA
		match pkt.set_flag1(0x08).send_recv(Duration::from_secs(10)) {
			Err(_) => {
				println!("Error sending packet to get OLT DNA");
				return;
			}
			Ok(res) => {
				let end_idx = res
					.data
					.iter()
					.rposition(|&b| !b.is_ascii_control())
					.map(|i| i + 1)
					.unwrap_or(0);
				let dna = hex::encode(&res.data[..end_idx]);
				{
					let mut olt = olt_arc.lock().unwrap();
					olt.olt_dna = dna.clone();
					println!("OLT DNA: {:}", dna);
				}
			}
		};

		// Start olt update
		let pcap_clone = pcap.clone();
		let olt_arc_clone = olt_arc.clone();
		std::thread::spawn(move || Olt::olt_update(olt_arc_clone, mac_dst, pcap_clone));

		let pkt = packets::Packet::new()
			.set_pcap(pcap.clone())
			.set_mac(mac_dst.clone())
			.set_request_type(0x000c)
			.set_flag0(0x02)
			.set_flag1(0x01);

		loop {
			for onu_id in 0..max_onu {
				let pkt = pkt.set_flag2(onu_id);
				match pkt.send_recv(Duration::from_secs(8)) {
					Err(_) => continue,
					Ok(res) => {
						let olt_uptime = { olt_arc.lock().unwrap().uptime };
						let status = ONUStatus::from(res.data[0]);
						{
							let olt = olt_arc.lock().unwrap();
							olt.onus.lock().unwrap()[onu_id as usize].set_status(status)
						};
						match status {
							ONUStatus::Online | ONUStatus::Omci => {
								let sn = match pkt.set_flag1(0x06).send_recv(Duration::from_secs(10)) {
									Err(_) => continue,
									Ok(res) => match gpon_sn::Sn::new(&res.data[..8]) {
										Ok(sn) => sn,
										Err(err) => {
											println!("Error parsing SN {:}", err);
											continue;
										}
									},
								};

								let uptime = match pkt.set_flag1(0x02).send_recv(Duration::from_secs(10)) {
									Err(_) => continue,
									Ok(res) => {
										let uptime = olt_uptime
											.saturating_sub(Duration::from_nanos(
												BigEndian::read_u64(&res.data[..8]) * 16,
											))
											.max(Duration::from_secs(0));
										uptime
									}
								};

								{
									let olt = olt_arc.lock().unwrap();
									let mut onus = olt.onus.lock().unwrap();
									onus[onu_id as usize].set_sn(sn);
									onus[onu_id as usize].set_uptime(uptime);
								};
							}
							_ => {
								// Update uptime and status
								{
									let olt = olt_arc.lock().unwrap();
									let mut onus = olt.onus.lock().unwrap();
									onus[onu_id as usize].set_sn(Sn::default());
									onus[onu_id as usize].set_uptime(Duration::new(0, 0));
								};
								continue;
							}
						}
					}
				}
			}
			sleep(Duration::from_secs(1));
		}
	}

	fn olt_update(olt_arc: Arc<Mutex<Olt>>, mac_dst: macaddr::MacAddr6, pcap: PcapShare) {
		let pkt = Packet::new()
			.set_pcap(pcap.clone())
			.set_mac(mac_dst.clone())
			.set_request_type(0x000c)
			.set_flag2(0xff);

		//?
		let _ = pkt
			.set_flag0(0x02)
			.set_flag1(0x01)
			.set_flag2(0x00)
			.send_recv(Duration::from_secs(10))
			.unwrap();

		// OLT Config
		let _ = pkt
			.set_request_type(0x00f)
			.set_flag1(0x09)
			.set_flag2(0xff)
			.send_recv(Duration::from_secs(10))
			.unwrap();

		loop {
			// Current Uptime
			match pkt.set_flag1(0x01).send_recv(Duration::from_secs(10)) {
				Err(_) => continue,
				Ok(res) => {
					let mut olt = olt_arc.lock().unwrap();
					let s2 = Duration::from_secs(2);
					let s1 = Duration::from_secs(0);
					olt.uptime =
						(Duration::from_nanos(BigEndian::read_u64(&res.data[..8]) * 16) - s2).max(s1);
				}
			}

			// Current ONUs online
			match pkt.set_flag0(0x02).send_recv(Duration::from_secs(10)) {
				Err(_) => continue,
				Ok(res) => {
					let mut olt = olt_arc.lock().unwrap();
					olt.online_onu = res.data[0];
				}
			}

			// Get Current OMCI Mode
			match pkt
				.set_flag0(0x01)
				.set_flag1(0x19)
				.send_recv(Duration::from_secs(10))
			{
				Err(_) => continue,
				Ok(res) => {
					let mut olt = olt_arc.lock().unwrap();
					olt.omci_mode = res.data[0];
				}
			}

			// Get Current Temperature
			match pkt.set_flag1(0x03).send_recv(Duration::from_secs(10)) {
				Err(_) => continue,
				Ok(res) => {
					let mut olt = olt_arc.lock().unwrap();
					olt.temperature = ((BigEndian::read_u16(&res.data[..2]) as f64) / 100.0) * 2.0;
				}
			}

			// Get Max Temperature
			match pkt.set_flag1(0x04).send_recv(Duration::from_secs(10)) {
				Err(_) => continue,
				Ok(res) => {
					let mut olt = olt_arc.lock().unwrap();
					olt.max_temperature = ((BigEndian::read_u16(&res.data[..2]) as f64) / 100.0) * 2.0;
				}
			}

			sleep(Duration::from_secs(1));
		}
	}

	// Equivalente ao `SetAuthPass` do Golang
	pub fn build_auth_pass_pkt(&self, pass: &str) -> Packet {
		Packet::new()
			.set_mac(self.mac_addr)
			.set_request_type(0x0014)
			.set_flag3(0x02)
			.set_flag0(0x01)
			.set_flag1(0x0c)
			.set_flag2(0xff)
			.set_data_vec(pass.as_bytes().to_vec())
	}

	// Builder para polling das infos periódicas da OLT (equivalente à rotina `OltInfo`)
	pub fn build_olt_info_queries(&self) -> Vec<Packet> {
		let base_pkt = Packet::new()
			.set_mac(self.mac_addr)
			.set_request_type(0x000c)
			.set_flag2(0xff);

		vec![
			base_pkt.clone().set_flag1(0x03), // Temperatura atual
			base_pkt.clone().set_flag1(0x04), // Temperatura máxima
			base_pkt.clone().set_flag0(0x02), // Online ONUs
			base_pkt.clone().set_flag1(0x01), // OLT Uptime
		]
	}

	/// Recebe o pacote e atualiza os parâmetros baseando-se nas flags (Tradução do OltInfo e fetchONUInfo)
	pub fn process_packet(&mut self, _pkt: &Packet) {}
}
