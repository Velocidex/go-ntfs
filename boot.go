package ntfs

import (
	"errors"
	"fmt"
	"io"

	"www.velocidex.com/golang/vtypes"
)

type NTFS_BOOT_SECTOR struct {
	vtypes.Object

	cluster_size int64
}

func (self *NTFS_BOOT_SECTOR) ClusterSize() int64 {
	// This is frequently used so we can cache it locally.
	if self.cluster_size == 0 {
		self.cluster_size = self.Get("_cluster_size").AsInteger() *
			self.Get("sector_size").AsInteger()
	}

	return self.cluster_size
}

func (self *NTFS_BOOT_SECTOR) BlockCount() int64 {
	return self.Get("_volume_size").AsInteger() / self.ClusterSize()
}

func (self *NTFS_BOOT_SECTOR) RecordSize() int64 {
	_record_size := self.Get("_mft_record_size").AsInteger()
	if _record_size > 0 {
		return _record_size * self.ClusterSize()
	}
	return 1 << uint32(-_record_size)
}

func (self *NTFS_BOOT_SECTOR) MFT() (*MFT_ENTRY, error) {
	// The MFT is a table of MFT_ENTRY records read from an
	// abstracted reader which is itself a $DATA attribute of the
	// first MFT record:

	// MFT[0] -> Attr $DATA contains the entire $MFT stream.

	// We therefore need to bootstrap the MFT:
	// 1. Read the first entry in the first cluster using the disk reader.
	// 2. Search for the $DATA attribute.
	// 3. Reconstruct the runlist and RunReader from this attribute.
	// 4. Instantiate the MFT over this new reader.
	offset := self.Get("_mft_cluster").AsInteger() * self.ClusterSize()
	root_entry, err := _NewMFTEntry(self, self.Reader(), offset)
	if err != nil {
		return nil, err
	}

	for _, attr := range root_entry.Attributes() {
		// $DATA attribute = 128.
		if attr.Get("type").AsInteger() == 128 {
			result, err := _NewMFTEntry(self, attr.Data(), 0)
			return result, err
		}
	}

	return nil, errors.New("$DATA attribute not found for $MFT")
}

// NTFS Parsing starts with the boot record.
func NewBootRecord(profile *vtypes.Profile, reader io.ReaderAt, offset int64) (
	*NTFS_BOOT_SECTOR, error) {

	record_obj, err := profile.Create("NTFS_BOOT_SECTOR", offset, reader, nil)
	if err != nil {
		return nil, err
	}

	self := &NTFS_BOOT_SECTOR{Object: record_obj}
	if self.Get("magic").AsInteger() != 0xaa55 {
		return self, errors.New("Invalid magic")
	}

	switch self.ClusterSize() {
	case 0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40,
		0x80, 0x100, 0x200, 0x400, 0x800, 0x1000:
		break
	default:
		return self, errors.New(
			fmt.Sprintf("Invalid cluster size %x",
				self.ClusterSize()))
	}

	sector_size := self.Get("sector_size").AsInteger()
	if sector_size == 0 || (sector_size%512 != 0) {
		return self, errors.New("Invalid sector_size")
	}

	if self.BlockCount() == 0 {
		return self, errors.New("Volume size is 0")
	}

	return self, nil
}

func GetProfile() (*vtypes.Profile, error) {
	profile := vtypes.NewProfile()
	err := profile.ParseStructDefinitions(NTFS_PROFILE)
	if err != nil {
		return nil, err
	}
	vtypes.AddModel(profile)

	// Add our local parsers.
	AddParsers(profile)

	return profile, nil
}
