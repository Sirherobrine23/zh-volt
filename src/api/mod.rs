pub mod http;
pub mod unix;

use std::str::FromStr;

pub use tiny_http::{Header, Response, Server};

use crate::olt::olt_maneger::SharedOltState;

pub enum ErrorListen {
	Unix(String),
	Http(String),
}

pub fn bootapi(listeners: Vec<String>, shared_olts: SharedOltState) -> Option<ErrorListen> {
	for listen in listeners {
		let shared_olts = shared_olts.clone();
		// Check if listen unix socket or http server
		if listen.starts_with("unix:") || listen.ends_with(".sock") {
			let listen = listen.replace("unix:", "");
			match unix::create_unix_listen(&listen) {
				Err(err) => return Some(ErrorListen::Unix(err.to_string())),
				Ok(listen) => {
					println!(
						"Starting Unix Socket API on {}",
						listen
							.local_addr()
							.unwrap()
							.as_pathname()
							.unwrap()
							.to_str()
							.unwrap()
					);
					std::thread::spawn(move || unix::process(listen, shared_olts.clone()))
				}
			};
		} else {
			match Server::http(listen) {
				Err(err) => return Some(ErrorListen::Http(err.to_string())),
				Ok(server) => {
					println!("Starting HTTP API on {}", server.server_addr());
					std::thread::spawn(move || http::process(server, shared_olts.clone()))
				}
			};
		}
	}

	None
}

#[derive(PartialEq, Debug)]
pub enum StatusOutputType {
	Json,
	Text,
}

impl FromStr for StatusOutputType {
	type Err = String;

	fn from_str(s: &str) -> Result<Self, Self::Err> {
		match s {
			"json" => Ok(StatusOutputType::Json),
			"text" => Ok(StatusOutputType::Text),
			_ => Err(format!("invalid output type: {}", s)),
		}
	}
}
