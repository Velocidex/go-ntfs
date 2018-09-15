package ntfs

import (
	"errors"

	"www.velocidex.com/golang/vtypes"
)

type NTFS_BOOT_SECTOR struct {
	vtypes.Object
}

func (self *NTFS_BOOT_SECTOR) ClusterSize() int64 {
	return self.Get("_cluster_size").AsInteger() *
		self.Get("sector_size").AsInteger()
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
