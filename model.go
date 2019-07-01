package ntfs

import "time"

// This file defines a model for MFT entry.

type TimeStamps struct {
	CreateTime       time.Time
	FileModifiedTime time.Time
	MFTModifiedTime  time.Time
	AccessedTime     time.Time
}

type FilenameInfo struct {
	Times TimeStamps
	Type  string
	Name  string
}

type NTFSFileInformation struct {
	FullPath  string
	MFTID     int64
	Size      int64
	Allocated bool
	IsDir     bool
	SI_Times  *TimeStamps
	Filenames []*FilenameInfo
}

func ModelMFTEntry(mft_entry *MFT_ENTRY) (*NTFSFileInformation, error) {
	full_path, _ := GetFullPath(mft_entry)
	result := &NTFSFileInformation{
		FullPath:  full_path,
		MFTID:     mft_entry.Get("record_number").AsInteger(),
		Allocated: mft_entry.Get("flags").Get("ALLOCATED").AsInteger() != 0,
		IsDir:     mft_entry.Get("flags").Get("DIRECTORY").AsInteger() != 0,
	}

	si, err := mft_entry.StandardInformation()
	if err == nil {
		result.SI_Times = &TimeStamps{
			CreateTime: time.Unix(si.Get("create_time").
				AsInteger(), 0),
			FileModifiedTime: time.Unix(si.Get("file_altered_time").
				AsInteger(), 0),
			MFTModifiedTime: time.Unix(si.Get("mft_altered_time").
				AsInteger(), 0),
			AccessedTime: time.Unix(si.Get("file_accessed_time").
				AsInteger(), 0),
		}
	}

	for _, filename := range mft_entry.FileName() {
		result.Filenames = append(result.Filenames, &FilenameInfo{
			Times: TimeStamps{
				CreateTime: time.Unix(
					filename.Get("created").AsInteger(), 0),
				FileModifiedTime: time.Unix(
					filename.Get("file_modified").AsInteger(), 0),
				MFTModifiedTime: time.Unix(
					filename.Get("mft_modified").AsInteger(), 0),
				AccessedTime: time.Unix(
					filename.Get("file_accessed").AsInteger(), 0),
			},
			Type: filename.Get("name_type").AsString(),
			Name: filename.Name(),
		})
	}

	for _, attr := range mft_entry.Attributes() {
		// $DATA attribute = 128.
		if attr.Get("type").AsInteger() == 128 {
			if attr.IsResident() {
				result.Size = attr.Get("content_size").AsInteger()
			} else {
				result.Size = attr.Get("actual_size").AsInteger()
			}
			break
		}
	}

	return result, nil
}
