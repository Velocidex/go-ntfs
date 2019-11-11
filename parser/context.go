package parser

import (
	"errors"
	"io"
)

type NTFSContext struct {
	DiskReader  io.ReaderAt
	Boot        *NTFS_BOOT_SECTOR
	RootMFT     *MFT_ENTRY
	Profile     *NTFSProfile
	ClusterSize int64
	RecordSize  int64
}

func (self *NTFSContext) GetRecordSize() int64 {
	if self.RecordSize == 0 {
		self.RecordSize = self.Boot.RecordSize()
	}

	return self.RecordSize
}

func (self *NTFSContext) GetMFT(id int64) (*MFT_ENTRY, error) {
	// The root MFT is read from the $MFT stream so we can just
	// reuse its reader.
	if self.RootMFT == nil {
		return nil, errors.New("No RootMFT known.")
	}

	disk_mft := self.Profile.MFT_ENTRY(self.RootMFT.Reader,
		self.GetRecordSize()*id)

	// Fixup the entry.
	mft_reader, err := FixUpDiskMFTEntry(disk_mft)
	if err != nil {
		return nil, err
	}

	return self.Profile.MFT_ENTRY(mft_reader, 0), nil
}
