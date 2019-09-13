package parser

import (
	"io"
)

type NTFSContext struct {
	DiskReader io.ReaderAt
	Boot       *NTFS_BOOT_SECTOR
	RootMFT    *MFT_ENTRY
	Profile    *NTFSProfile
}

func (self *NTFSContext) GetMFT(id int64) (*MFT_ENTRY, error) {
	// The root MFT is read from the $MFT stream so we can just
	// reuse its reader.
	disk_mft := self.Profile.MFT_ENTRY(self.RootMFT.Reader,
		self.Boot.RecordSize()*id)

	// Fixup the entry.
	mft_reader, err := FixUpDiskMFTEntry(disk_mft)
	if err != nil {
		return nil, err
	}

	return self.Profile.MFT_ENTRY(mft_reader, 0), nil
}
