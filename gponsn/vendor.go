package gponsn

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrVendorNullSn  = errors.New("null sn")
	ErrVendorInvalid = errors.New("invalid GPON SN")
	ErrVendorName    = errors.New("invalid vendor name")
)

// GPON Vendor IDs, from https://hack-gpon.org/vendor/
type Vendor uint32

const (
	VENDOR_ADTN = Vendor(0x4144544e)
	VENDOR_ALCL = Vendor(0x414c434c)
	VENDOR_ALLG = Vendor(0x414c4c47)
	VENDOR_AVMG = Vendor(0x41564d47)
	VENDOR_ASKY = Vendor(0x41534b59)
	VENDOR_CDKT = Vendor(0x43444B54)
	VENDOR_CIGG = Vendor(0x43494747)
	VENDOR_CXNK = Vendor(0x43584e4b)
	VENDOR_DDKT = Vendor(0x44444b54)
	VENDOR_DLNK = Vendor(0x444c4e4b)
	VENDOR_DSNW = Vendor(0x44534e57)
	VENDOR_ELTX = Vendor(0x454c5458)
	VENDOR_FHTT = Vendor(0x46485454)
	VENDOR_GMTK = Vendor(0x474d544b)
	VENDOR_GNXS = Vendor(0x474e5853)
	VENDOR_GPNC = Vendor(0x47504E43)
	VENDOR_GPON = Vendor(0x47504f4e)
	VENDOR_GTHG = Vendor(0x47544847)
	VENDOR_HALN = Vendor(0x48414c4e)
	VENDOR_HBMT = Vendor(0x48424d54)
	VENDOR_HUMA = Vendor(0x48554d41)
	VENDOR_HWTC = Vendor(0x48575443)
	VENDOR_ICTR = Vendor(0x49435452)
	VENDOR_ISKT = Vendor(0x49534b54)
	VENDOR_KAON = Vendor(0x4b414f4e)
	VENDOR_LEOX = Vendor(0x4c454f58)
	VENDOR_LQDE = Vendor(0x4c514445)
	VENDOR_NOKG = Vendor(0x4e4f4b47)
	VENDOR_NOKW = Vendor(0x4e4f4b57)
	VENDOR_MSTC = Vendor(0x4d535443)
	VENDOR_PTIN = Vendor(0x5054494e)
	VENDOR_RTKG = Vendor(0x52544b47)
	VENDOR_SCOM = Vendor(0x53434f4d)
	VENDOR_SKYW = Vendor(0x534b5957)
	VENDOR_SMBS = Vendor(0x534d4253)
	VENDOR_SPGA = Vendor(0x53504741)
	VENDOR_TMBB = Vendor(0x544d4242)
	VENDOR_TPLG = Vendor(0x54504c47)
	VENDOR_UBNT = Vendor(0x55424e54)
	VENDOR_UGRD = Vendor(0x55475244)
	VENDOR_YHTC = Vendor(0x59485443)
	VENDOR_ZNTS = Vendor(0x5a4e5453)
	VENDOR_ZRMT = Vendor(0x5a524d54)
	VENDOR_ZTEG = Vendor(0x5a544547)
	VENDOR_ZYWN = Vendor(0x5a59574e)
	VENDOR_ZYXE = Vendor(0x5a595845)
)

var (
	vendorNames = map[Vendor]string{
		VENDOR_ADTN: "Adtran",
		VENDOR_ALCL: "Nokia/Alcatel-Lucent",
		VENDOR_ALLG: "ALLNET",
		VENDOR_AVMG: "AVM (FRITZ!Box)",
		VENDOR_ASKY: "Askey",
		VENDOR_CDKT: "KingType",
		VENDOR_CIGG: "Cig",
		VENDOR_CXNK: "Calix",
		VENDOR_DDKT: "DKT",
		VENDOR_DLNK: "Dlink",
		VENDOR_DSNW: "DASAN",
		VENDOR_ELTX: "Eltex",
		VENDOR_FHTT: "FiberHome",
		VENDOR_GMTK: "GemTek",
		VENDOR_GNXS: "Genexis",
		VENDOR_GPNC: "NuCom",
		VENDOR_GPON: "Generic vendor name",
		VENDOR_GTHG: "Alcatel-Lucent (ODM)",
		VENDOR_HALN: "HALNy",
		VENDOR_HBMT: "HiSense",
		VENDOR_HUMA: "Humax",
		VENDOR_HWTC: "Huawei",
		VENDOR_ICTR: "Icotera",
		VENDOR_ISKT: "Iskratel",
		VENDOR_KAON: "KAONMEDIA",
		VENDOR_LEOX: "LEOX",
		VENDOR_LQDE: "Lantiq",
		VENDOR_NOKG: "Nokia (GemTek ODM)",
		VENDOR_NOKW: "Nokia (GemTek ODM)",
		VENDOR_MSTC: "Mitrastar",
		VENDOR_PTIN: "Altice/PT Inovação",
		VENDOR_RTKG: "Realtek",
		VENDOR_SCOM: "Sercomm",
		VENDOR_SKYW: "Skyworth",
		VENDOR_SMBS: "Sagemcom",
		VENDOR_SPGA: "SourcePhotonics",
		VENDOR_TMBB: "Technicolor",
		VENDOR_TPLG: "TP-Link",
		VENDOR_UBNT: "Ubiquiti",
		VENDOR_UGRD: "UGrid",
		VENDOR_YHTC: "Youhua",
		VENDOR_ZNTS: "DZS",
		VENDOR_ZRMT: "Zaram",
		VENDOR_ZTEG: "ZTE",
		VENDOR_ZYWN: "Zyxel",
		VENDOR_ZYXE: "Zyxel",
	}
)

func (vendor Vendor) String() string {
	return fmt.Sprintf("%x", uint32(vendor))
}

func (vendor Vendor) HexString() string {
	if vendor == 0 {
		return "0000"
	}
	return string(binary.BigEndian.AppendUint32(nil, uint32(vendor)))
}

func (vendor Vendor) IsValid() bool {
	_, ok := vendorNames[vendor]
	return ok
}

func (vendor Vendor) MarshalText() ([]byte, error) {
	if vendor.IsValid() {
		return fmt.Appendf(nil, "%x", vendor), nil
	}
	return nil, ErrVendorInvalid
}

func (vendor Vendor) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 0, 8)
	binary.BigEndian.PutUint32(buf, uint32(vendor))
	return buf, nil
}

func (vendor *Vendor) UnmarshalBinary(data []byte) (err error) {
	switch len(data) {
	case 4:
		odata := data
		if data, err = hex.DecodeString(string(data)); err != nil {
			data = binary.BigEndian.AppendUint32(nil, uint32(binary.BigEndian.Uint32(odata)))
		}
		fallthrough
	case 8:
		decodeVendor := Vendor(binary.BigEndian.Uint32(data))
		if _, ok := vendorNames[decodeVendor]; ok {
			*vendor = decodeVendor
			return nil
		}
	default:
		return ErrVendorNullSn
	}

	return ErrVendorNullSn
}

func (vendor *Vendor) UnmarshallText(data []byte) error {
	if len(data) < 4 {
		return ErrVendorName
	}

	ver := Vendor(binary.BigEndian.Uint32(data[:4]))
	if _, ok := vendorNames[ver]; ok {
		*vendor = ver
		return nil
	}

	for ver, name := range vendorNames {
		if strings.Contains(string(data), name) {
			*vendor = ver
			return nil
		}
	}

	return ErrVendorName
}

func IsVendorVaid(data []byte) bool {
	var sn Vendor
	return sn.UnmarshalBinary(data) == nil
}