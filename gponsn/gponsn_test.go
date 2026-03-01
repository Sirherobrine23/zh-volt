package gponsn_test

import (
	"testing"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/zhvolt/gponsn"
)

var GPONs = []struct {
	SN     string
	Vendor gponsn.Vendor
	ID     uint32
}{
	{"MSTC039eacf5", gponsn.VENDOR_MSTC, 0x039eacf5},
	{"LLLL009eacf5", 0, 0x039eacf5},
	{"TPLG89bdcfd8", gponsn.VENDOR_TPLG, 0x89bdcfd8},
	{"TPLG00000000", gponsn.VENDOR_TPLG, 0x89bdcfd8},
	{"MSTC-039eacf5", gponsn.VENDOR_MSTC, 0x039eacf5},
	{"LLLL009eacf5", 0, 0x039eacf5},
	{"4d535443039eacf5", gponsn.VENDOR_MSTC, 0x039eacf5},
}

func TestGponSN(t *testing.T) {
	for index, data := range GPONs {
		var sn gponsn.Sn
		err := sn.UnmarshalText([]byte(data.SN))
		if err != nil && (index%2 == 0) {
			t.Errorf("cannot unmarshall valid text %d: %s", index, err)
			return
		} else if err == nil && (index%2 != 0) {
			t.Errorf("invalid data processwd without error %d", index)
			return
		} else if index%2 != 0 {
			continue
		}

		if sn.Vendor() != data.Vendor {
			t.Errorf("unmarshall get invalid vendor from text %d: 0x%x ≃ 0%x", index, uint32(sn.Vendor()), data.Vendor)
			return
		} else if sn.ID() != data.ID {
			t.Errorf("unmarshall get invalid id from text %d, %d != %d", index, sn.ID(), data.ID)
			return
		}

		bin, err := sn.MarshalBinary()
		if err != nil {
			t.Errorf("error in encode GPONSN: %s", err)
			return
		}

		txt, err := sn.MarshalText()
		if err != nil {
			t.Errorf("error in encode GPONSN: %s", err)
			return
		}

		t.Logf("SN: String %s, Bin: %x, Txt: %s", sn.String(), string(bin), string(txt))
	}
}
