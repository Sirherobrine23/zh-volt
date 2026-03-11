pub mod api;
pub mod olt;
pub mod sn;

use clap::{Command, arg};
use tiny_http::Server;

use crate::olt::olt_maneger::{OltManager, new_pcap_dev, new_share};

fn main() {
	let cmd = Command::new("zh-volt")
		.arg(arg!(-i --netdev <String> "Net device to watch packets").default_value("eth0"))
		.arg(arg!(-l --http_listen <String> "HTTP api listen").default_value("0.0.0.0:8081"))
		.get_matches();

	let net_dev = cmd.get_one::<String>("netdev").unwrap().to_string();
	let addr = cmd.get_one::<String>("http_listen").unwrap().to_string();
	let server = Server::http(addr).unwrap();

	let shared_olts = new_share();
	let htt_shared_olts = shared_olts.clone();
	let dev = match new_pcap_dev(net_dev) {
		Err(err) => panic!("Error starting Pcap: {:?}", err),
		Ok(dev) => dev,
	};

	std::thread::spawn(move || api::route::create_router(server, htt_shared_olts));
	let mut manager = OltManager::new(dev, shared_olts);
	let man = std::thread::spawn(move || manager.run());

	let _ = man.join().unwrap();
}
