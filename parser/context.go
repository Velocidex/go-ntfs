package parser

import (
	"errors"
	"fmt"
	"io"
)

type NTFSContext struct {
	DiskReader  io.ReaderAt
	Boot        *NTFS_BOOT_SECTOR
	RootMFT     *MFT_ENTRY
	Profile     *NTFSProfile
	ClusterSize int64
	RecordSize  int64

	// Map MFTID to *MFT_ENTRY
	mft_entry_lru *LRU
}

func newNTFSContext(image io.ReaderAt) *NTFSContext {
	STATS.Inc_NTFSContext()
	mft_cache, _ := NewLRU(20000, nil)
	return &NTFSContext{
		DiskReader:    image,
		Profile:       NewNTFSProfile(),
		mft_entry_lru: mft_cache,
	}
}

func (self *NTFSContext) Close() {
	if debug {
		fmt.Printf(STATS.DebugString())
		fmt.Println(self.mft_entry_lru.DebugString())
	}
	self.mft_entry_lru.Purge()
}

func (self *NTFSContext) Purge() {
	self.mft_entry_lru.Purge()
}

func (self *NTFSContext) GetRecordSize() int64 {
	if self.RecordSize == 0 {
		self.RecordSize = self.Boot.RecordSize()
	}

	return self.RecordSize
}

func (self *NTFSContext) GetMFT(id int64) (*MFT_ENTRY, error) {
	// Check the cache first
	cached_any, pres := self.mft_entry_lru.Get(int(id))
	if pres {
		return cached_any.(*MFT_ENTRY), nil
	}

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

	result := self.Profile.MFT_ENTRY(mft_reader, 0)
	self.mft_entry_lru.Add(int(id), result)
	return result, nil
}
