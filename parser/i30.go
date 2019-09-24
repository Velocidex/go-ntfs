package parser

import "fmt"

func ExtractI30List(ntfs *NTFSContext, mft_entry *MFT_ENTRY) []*FileInfo {
	records := []*INDEX_RECORD_ENTRY{}

	for _, node := range mft_entry.DirNodes(ntfs) {
		records = append(records, node.GetRecords(ntfs)...)
		records = append(records, node.ScanSlack(ntfs)...)
	}

	result := []*FileInfo{}
	for _, record := range records {
		if !record.IsValid() {
			continue
		}

		filename := record.File()
		result = append(result, &FileInfo{
			MFTId:    fmt.Sprintf("%d", record.MftReference()),
			Mtime:    filename.File_modified().Time,
			Atime:    filename.File_accessed().Time,
			Ctime:    filename.Mft_modified().Time,
			Name:     filename.Name(),
			NameType: filename.NameType().Name,
		})
	}

	return result
}

const (
	earliest_valid_time = 1000000000 // Sun Sep  9 11:46:40 2001
	latest_valid_time   = 2000000000 // Wed May 18 13:33:20 2033
)

func (self *INDEX_RECORD_ENTRY) IsValid() bool {
	test_filename := self.File()

	x := test_filename.File_modified().Unix()
	if x < earliest_valid_time || x > latest_valid_time {
		return false
	}

	x = test_filename.File_accessed().Unix()
	if x < earliest_valid_time || x > latest_valid_time {
		return false
	}

	x = test_filename.Mft_modified().Unix()
	if x < earliest_valid_time || x > latest_valid_time {
		return false
	}

	x = test_filename.Created().Unix()
	if x < earliest_valid_time || x > latest_valid_time {
		return false
	}

	return true
}

func (self *INDEX_NODE_HEADER) ScanSlack(ntfs *NTFSContext) []*INDEX_RECORD_ENTRY {
	result := []*INDEX_RECORD_ENTRY{}

	// start at the last record and carve until the end of the
	// allocation.
	start := int32(self.Offset_to_end_index_entry())
	end := self.SizeOfEntriesAlloc() - 0x52
	for off := start; off < end; off++ {
		test_struct := self.Profile.INDEX_RECORD_ENTRY(self.Reader, int64(off))
		if test_struct.IsValid() {
			result = append(result, test_struct)
		}
	}

	return result
}
