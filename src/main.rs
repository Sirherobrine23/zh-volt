pub mod api;
pub mod olt;
pub mod sn;

use std::process::ExitCode;

use argh::FromArgs;

use crate::api::{ErrorListen, StatusOutputType, bootapi, unix};
use crate::olt::olt_maneger::{OltManager, new_pcap_dev, new_share};
use crate::olt::pcap::ErrPcap;

#[derive(FromArgs, PartialEq, Debug)]
#[argh(subcommand, name = "daemon")]
/// Run daemon for manager zh-volt olt's
struct Daemon {
	#[argh(option, short = 'i')]
	/// network device to manager OLTs
	netdev: Option<String>,

	#[argh(option, short = 'l')]
	/// HTTP api or socket to listen
	listen: Vec<String>,
}

#[derive(FromArgs, PartialEq, Debug)]
#[argh(subcommand, name = "status")]
/// Print OLT info
struct Status {
	#[argh(switch, short = 'w')]
	/// watch OLT info
	watch: Option<bool>,

	#[argh(option, short = 'o')]
	/// output type
	output: Option<StatusOutputType>,
}

#[derive(FromArgs, PartialEq, Debug)]
#[argh(subcommand)]
enum ManegerSubCommands {
	Status(Status),
}

#[derive(FromArgs, PartialEq, Debug)]
#[argh(subcommand, name = "maneger")]
/// Interact with the daemon
struct Manager {
	#[argh(option, short = 'c')]
	/// client connection
	connect: Option<String>,

	#[argh(subcommand)]
	nested: ManegerSubCommands,
}

#[derive(FromArgs, PartialEq, Debug)]
#[argh(subcommand)]
enum SubCommands {
	Daemon(Daemon),
	Cli(Manager),
}

#[derive(FromArgs, PartialEq, Debug)]
/// ZH Volt manager cli.
struct ZhVolt {
	#[argh(subcommand)]
	nested: SubCommands,
}

fn main() -> ExitCode {
	let args: ZhVolt = argh::from_env();

	match args.nested {
		SubCommands::Daemon(daemon_options) => {
			let shared_olts = new_share();

			if daemon_options.listen.len() == 0 {
				eprintln!("Add at least one listener");
				return ExitCode::from(2);
			}

			// Start API or Unix Listen
			match bootapi(daemon_options.listen, shared_olts.clone()) {
				None => (),
				Some(ErrorListen::Http(v)) => {
					eprintln!("Error starting HTTP API: {}", v);
					return ExitCode::FAILURE;
				}
				Some(ErrorListen::Unix(v)) => {
					eprintln!("Error starting Unix Socket API: {}", v);
					return ExitCode::FAILURE;
				}
			}

			// Start Manager
			let dev = match new_pcap_dev(daemon_options.netdev.unwrap_or("eth0".to_string())) {
				Ok(dev) => dev,
				Err(ErrPcap::ErrCannotSend) => return ExitCode::FAILURE,
				Err(ErrPcap::ErrCannotRecive) => return ExitCode::FAILURE,
				Err(ErrPcap::LockError) => return ExitCode::FAILURE,
				Err(ErrPcap::Timeout) => return ExitCode::FAILURE,
				Err(ErrPcap::Packet(_)) => return ExitCode::FAILURE,
				Err(ErrPcap::ErrNoDevice) => {
					eprintln!("Error starting manager: Network interface not found");
					return ExitCode::FAILURE;
				}
				Err(ErrPcap::Io(err)) => {
					eprintln!("Error starting manager: {}", err);
					return ExitCode::FAILURE;
				}
			};
			let mut manager = OltManager::new(dev, shared_olts.clone());
			let _ = std::thread::spawn(move || manager.run()).join().unwrap();
		}
		SubCommands::Cli(opts) => {
			let connection_str = opts.connect.unwrap_or(String::from(""));
			match opts.nested {
				ManegerSubCommands::Status(config) => {
					if connection_str == "" {
						println!("No connection string provided");
						return ExitCode::FAILURE;
					}
					let watch = config.watch.unwrap_or(false);
					let output = config.output.unwrap_or(StatusOutputType::Json);

					if connection_str.starts_with("unix:") || connection_str.ends_with(".sock") {
						let _ = unix::client_status(connection_str, watch, output);
					}
				}
			}
		}
	}

	ExitCode::SUCCESS
}
