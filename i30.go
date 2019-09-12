package ntfs

import (
	"time"
)

func ExtractI30List(mft_entry *MFT_ENTRY) []*FileInfo {
	records := []*INDEX_RECORD_ENTRY{}

	for _, node := range mft_entry.DirNodes() {
		records = append(records, node.GetRecords()...)
		records = append(records, node.ScanSlack()...)
	}

	result := []*FileInfo{}
	for _, record := range records {
		if !record.IsValid() {
			continue
		}

		filename := &FILE_NAME{record.Get("file")}
		result = append(result, &FileInfo{
			MFTId: record.Get("mftReference").AsString(),
			Mtime: time.Unix(
				filename.Get("file_modified").
					AsInteger(), 0),
			Atime: time.Unix(
				filename.Get("file_accessed").
					AsInteger(), 0),
			Ctime: time.Unix(
				filename.Get("mft_modified").
					AsInteger(), 0),
			Name:     filename.Name(),
			NameType: filename.Get("name_type").AsString(),
		})
	}

	return result
}

const (
	earliest_valid_time = 1000000000 // Sun Sep  9 11:46:40 2001
	latest_valid_time   = 2000000000 // Wed May 18 13:33:20 2033
)

func (self *INDEX_RECORD_ENTRY) IsValid() bool {
	test_filename := &FILE_NAME{self.Get("file")}

	for _, field := range []string{
		"file_modified", "file_accessed",
		"mft_modified", "created"} {
		test_time := test_filename.Get(field).AsInteger()
		if test_time < earliest_valid_time || test_time > latest_valid_time {
			return false
		}
	}

	return true
}

func (self *INDEX_NODE_HEADER) ScanSlack() []*INDEX_RECORD_ENTRY {
	result := []*INDEX_RECORD_ENTRY{}

	// start at the last record and carve until the end of the
	// allocation.
	start := self.Get("offset_to_end_index_entry").AsInteger()
	end := self.Get("sizeOfEntriesAlloc").AsInteger() - 0x52
	for off := start; off < end; off++ {
		test_struct_obj, _ := self.Profile().Create(
			"INDEX_RECORD_ENTRY", off, self.Reader(), nil)
		test_struct := &INDEX_RECORD_ENTRY{test_struct_obj}
		if test_struct.IsValid() {
			result = append(result, test_struct)
		}
	}

	return result
}
