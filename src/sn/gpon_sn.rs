use std::fmt::{Debug, Display};

use byteorder::{BigEndian, ByteOrder};
use hex;
use serde::{Deserialize, Serialize};

use crate::sn::vendor::GponVendors;

pub enum SnError {
	NoValidData,
	Hex(hex::FromHexError),
}

#[derive(Clone, Debug)]
pub struct Sn(pub [u8; 8]);

impl Sn {
	pub fn new(data: &[u8]) -> Result<Self, SnError> {
		match data.len() {
			8 => Ok(Sn(data.try_into().unwrap())),
			12 => {
				let mut buff = [0u8; 8];
				buff[0..4].copy_from_slice(&data[0..4]);
				hex::decode_to_slice(&data[4..12], &mut buff[4..8]).map_err(SnError::Hex)?;
				Ok(Sn(buff))
			}
			16 => {
				let mut buff = [0u8; 8];
				hex::decode_to_slice(&data[0..8], &mut buff[0..4]).map_err(SnError::Hex)?;
				hex::decode_to_slice(&data[8..16], &mut buff[4..8]).map_err(SnError::Hex)?;
				Ok(Sn(buff))
			}
			_ => Err(SnError::NoValidData.into()),
		}
	}

	pub fn is_valid(&self) -> bool {
		match self.vendor() {
			None => false,
			Some(_) => BigEndian::read_u32(&self.0[4..8]) > 0,
		}
	}

	pub fn vendor(&self) -> Option<GponVendors> {
		return GponVendors::decode(&self.0[0..4]);
	}

	pub fn id_string(&self) -> String {
		return hex::encode(&self.0[4..8]);
	}

	pub fn string(&self) -> String {
		if self.is_valid() {
			return format!(
				"{}-{}",
				self.vendor().unwrap().name().unwrap(),
				self.id_string()
			);
		}
		"0000-00000000".to_string()
	}

	pub fn from_string(s: &str) -> Result<Self, SnError> {
		if let Some((vendor, id)) = s.split_once('-') {
			return match GponVendors::from_str(vendor.to_string()) {
				None => Err(SnError::NoValidData.into()),
				Some(vendor) => {
					let mut buff = [0u8; 8];
					buff[0..4].copy_from_slice(&vendor.to_be_bytes());
					hex::decode_to_slice(id, &mut buff[4..8]).map_err(SnError::Hex)?;
					Ok(Sn(buff))
				}
			};
		} else if let Ok(buff) = hex::decode(s) {
			return Self::new(&buff);
		}
		Err(SnError::NoValidData.into())
	}
}

impl Default for Sn {
	fn default() -> Self {
		Sn([0; 8])
	}
}

impl Serialize for Sn {
	fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
	where
		S: serde::Serializer,
	{
		serializer.serialize_str(&self.string())
	}
}

impl<'de> Deserialize<'de> for Sn {
	fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
	where
		D: serde::Deserializer<'de>,
	{
		let s = String::deserialize(deserializer)?;
		if s.is_empty() {
			return Ok(Sn::default());
		}
		Ok(Sn::from_string(&s).unwrap())
	}
}

impl Debug for SnError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			SnError::NoValidData => write!(f, "No valid data"),
			SnError::Hex(err) => write!(f, "Hex error: {:?}", err),
		}
	}
}

impl Display for SnError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			SnError::NoValidData => write!(f, "No valid data"),
			SnError::Hex(err) => write!(f, "Hex error: {:?}", err),
		}
	}
}
