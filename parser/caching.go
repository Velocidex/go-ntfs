// Manage caching of MFT Entry metadata. This is mainly used for path
// traversal calculation.

package parser

import (
	"sync"

	"github.com/Velocidex/ordereddict"
)

type FNSummary struct {
	Name                 string
	NameType             string
	ParentEntryNumber    uint64
	ParentSequenceNumber uint16
}

type MFTEntrySummary struct {
	Sequence  uint16
	Filenames []FNSummary
}

type MFTEntryCache struct {
	mu sync.Mutex

	ntfs *NTFSContext

	lru *LRU

	preloaded map[uint64]*MFTEntrySummary
}

func (self *MFTEntryCache) Stats() *ordereddict.Dict {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.lru.Stats().Set("Preloaded", len(self.preloaded))
}

func NewMFTEntryCache(ntfs *NTFSContext) *MFTEntryCache {
	lru, _ := NewLRU(10000, nil, "MFTEntryCache")
	return &MFTEntryCache{
		ntfs:      ntfs,
		lru:       lru,
		preloaded: make(map[uint64]*MFTEntrySummary),
	}
}

// This function is used to preset persisted information in the cache
// about known MFT entries from other sources than the MFT itself. In
// particular, the USN journal is often a source of additional
// historical information. When resolving an MFT entry summary, we
// first look to the MFT itself, however if the sequence number does
// not match the required entry, we look toh the preloaded entry for a
// better match.
//
// The allows us to substitute historical information (from the USN
// journal) while resolving full paths.
func (self *MFTEntryCache) SetPreload(id uint64, seq uint16,
	cb func(entry *MFTEntrySummary) (*MFTEntrySummary, bool)) {
	key := id | uint64(seq)<<48

	// Optionally allow the callback to update the preloaded entry.
	entry, _ := self.preloaded[key]
	new_entry, updated := cb(entry)
	if updated {
		self.preloaded[key] = new_entry
	}
}

// GetSummary gets a MFTEntrySummary for the mft id. The sequence
// number is a hint for the required sequence of the entry. This
// function may return an MFTEntrySummary with a different sequence
// than requested.
func (self *MFTEntryCache) GetSummary(
	id uint64, seq uint16) (*MFTEntrySummary, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// We prefer to get the read entry from the MFT because it has all
	// the short names etc.
	res, err := self._GetSummary(id)
	if err != nil {
		return nil, err
	}

	// If the MFT entry is not correct (does not have the required
	// sequence number), we check the preloaded set for an approximate
	// match.
	if res.Sequence != seq {
		// Try to get from the preloaded records
		key := id | uint64(seq)<<48
		res, ok := self.preloaded[key]
		if ok {
			// Yep - the sequence number of correct.
			return res, nil
		}

		// Just return the incorrect entry - callers can add an error
		// for incorrect sequence number.
	}

	return res, nil
}

// Get the summary from the underlying MFT itself.
func (self *MFTEntryCache) _GetSummary(
	id uint64) (*MFTEntrySummary, error) {
	res_any, pres := self.lru.Get(int(id))
	if pres {
		res, ok := res_any.(*MFTEntrySummary)
		if ok {
			return res, nil
		}
	}

	mft_entry, err := self.ntfs.GetMFT(int64(id))
	if err != nil {
		return nil, err
	}

	cache_record := &MFTEntrySummary{
		Sequence: mft_entry.Sequence_value(),
	}
	for _, fn := range mft_entry.FileName(self.ntfs) {
		cache_record.Filenames = append(cache_record.Filenames,
			FNSummary{
				Name:                 fn.Name(),
				NameType:             fn.NameType().Name,
				ParentEntryNumber:    fn.MftReference(),
				ParentSequenceNumber: fn.Seq_num(),
			})
	}

	self.lru.Add(int(id), cache_record)
	return cache_record, nil
}
