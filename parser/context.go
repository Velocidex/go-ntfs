package parser

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

type NTFSContext struct {
	DiskReader io.ReaderAt
	Boot       *NTFS_BOOT_SECTOR
	RootMFT    *MFT_ENTRY
	Profile    *NTFSProfile

	MaxDirectoryDepth int

	ClusterSize int64

	mu         sync.Mutex
	RecordSize int64

	// Map MFTID to *MFT_ENTRY
	mft_entry_lru *LRU

	mft_summary_cache *MFTEntryCache
}

func newNTFSContext(image io.ReaderAt, name string) *NTFSContext {
	STATS.Inc_NTFSContext()
	mft_cache, _ := NewLRU(1000, nil, name)
	ntfs := &NTFSContext{
		DiskReader:        image,
		MaxDirectoryDepth: 20,
		Profile:           NewNTFSProfile(),
		mft_entry_lru:     mft_cache,
	}

	ntfs.mft_summary_cache = NewMFTEntryCache(ntfs)
	return ntfs
}

func (self *NTFSContext) Close() {
	if debug {
		fmt.Printf(STATS.DebugString())
		fmt.Println(self.mft_entry_lru.DebugString())
	}
	self.Purge()
}

func (self *NTFSContext) Purge() {
	self.mft_entry_lru.Purge()

	// Try to flush our reader if possible
	flusher, ok := self.DiskReader.(Flusher)
	if ok {
		flusher.Flush()
	}
}

func (self *NTFSContext) GetRecordSize() int64 {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.RecordSize == 0 {
		self.RecordSize = self.Boot.RecordSize()
	}

	return self.RecordSize
}

func (self *NTFSContext) GetMFTSummary(id uint64) (*MFTEntrySummary, error) {
	return self.mft_summary_cache.GetSummary(id)
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

	disk_mft := self.Profile.MFT_ENTRY(
		self.RootMFT.Reader, self.GetRecordSize()*id)

	// Fixup the entry.
	mft_reader, err := FixUpDiskMFTEntry(disk_mft)
	if err != nil {
		return nil, err
	}

	mft_entry := self.Profile.MFT_ENTRY(mft_reader, 0)
	self.mft_entry_lru.Add(int(id), mft_entry)

	return mft_entry, nil
}
