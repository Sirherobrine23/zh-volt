use std::str::FromStr;

use crate::olt::olt_maneger::{SharedOltState, get_olts_vec};
use serde_json::to_string_pretty;
use tiny_http::{Header, Response, Server};

pub fn process(server: Server, state: SharedOltState) {
	std::thread::spawn(move || {
		for req in server.incoming_requests() {
			let h1 = Header::from_str("Content-Type: application/json").unwrap();
			let h2 = Header::from_str("Access-Control-Allow-Origin: *").unwrap();

			match (req.method(), req.url()) {
				(tiny_http::Method::Get, "/") => {
					let data = match get_olts_vec(state.clone()) {
						Err(_) => {
							req
								.respond(
									Response::from_string("Error getting OLTs")
										.with_status_code(500)
										.with_header(h1)
										.with_header(h2),
								)
								.unwrap();
							return;
						}
						Ok(data) => data,
					};

					match to_string_pretty(&data) {
						Err(_) => {
							req
								.respond(
									Response::from_string("Error serializing JSON")
										.with_status_code(500)
										.with_header(h1)
										.with_header(h2),
								)
								.unwrap();
						}
						Ok(pretty_json) => {
							req
								.respond(
									Response::from_string(pretty_json)
										.with_status_code(200)
										.with_header(h1)
										.with_header(h2),
								)
								.unwrap();
						}
					};
				}
				_ => {
					req
						.respond(Response::empty(404).with_header(h1).with_header(h2))
						.unwrap();
				}
			}
		}
	});
}
