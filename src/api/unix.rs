use crate::api::StatusOutputType;
use crate::olt::olt_maneger::SharedOltState;
use crate::olt::olt_maneger::get_olts_vec;
use scanf::sscanf;
use serde_json::to_string;
use std::fs;
use std::io::Read;
use std::io::Write;
use std::os::unix::net::{UnixListener, UnixStream};
use std::time::Duration;

pub fn create_unix_listen(socket_path: &String) -> std::io::Result<UnixListener> {
	let _ = fs::remove_file(socket_path);
	UnixListener::bind(socket_path)
}

pub fn process(listener: UnixListener, state: SharedOltState) {
	for stream in listener.incoming() {
		match stream {
			Err(err) => println!("Connection failed: {}", err),
			Ok(mut stream) => {
				let state_clone = state.clone();
				std::thread::spawn(move || {
					let mut count: i64 = 1;
					let mut buffer = [0; 1024];
					let _ = stream.set_read_timeout(Some(Duration::from_millis(600)));
					if let Ok(size) = stream.read(&mut buffer) {
						let mut data: &str = std::str::from_utf8(&buffer[..size]).unwrap();
						if data.ends_with("\n") {
							data = data.trim_end_matches("\n");
						}
						if buffer.len() > 0 {
							if let Err(err) = sscanf!(data, "{}", &mut count) {
								println!("Error parsing count: {}", err);
							}
						}
					}

					while count == -1 || count > 0 {
						if count > 0 {
							count -= 1;
						}
						let data = match get_olts_vec(state_clone.clone()) {
							Err(_) => return (),
							Ok(data) => data,
						};

						let mut data_string = match to_string(&data) {
							Err(_) => return (),
							Ok(data) => data,
						};

						data_string.push_str("\n");
						if let Err(err) = stream.write_all(data_string.as_bytes()) {
							println!("Error writing to socket: {}", err);
							return;
						}
						if let Err(err) = stream.flush() {
							println!("Error flushing socket: {}", err);
						}

						std::thread::sleep(std::time::Duration::from_millis(300));
					}
				});
			}
		}
	}
}

pub fn client_status(
	conn: String,
	watch: bool,
	_output: StatusOutputType,
) -> Result<(), std::io::Error> {
	let mut stream = match UnixStream::connect(conn) {
		Err(err) => {
			println!("Error connecting to socket: {}", err);
			return Err(err);
		}
		Ok(stream) => stream,
	};

	if watch {
		let count: i64 = -1;
		let _ = stream.write(count.to_string().as_bytes());
	}

	loop {
		let mut buffer = [0; (1024 ^ 2) * 24];
		match stream.read(&mut buffer) {
			Err(err) => {
				println!("Error reading from socket: {}", err);
				return Err(err);
			}
			Ok(size) => {
				if size == 0 {
					break;
				}
				let data = std::str::from_utf8(&buffer[..size]).unwrap();
				print!("{}", data);
			}
		}
		if !watch {
			break;
		}
	}

	Ok(())
}
