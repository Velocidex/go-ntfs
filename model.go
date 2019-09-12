package ntfs

import (
	"fmt"
	"time"
)

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

type Attribute struct {
	Type   string
	TypeId int64
	Id     int64
	Inode  string
	Size   int64
	Name   string
}

// Describe a single MFT entry.
type NTFSFileInformation struct {
	FullPath  string
	MFTID     int64
	Size      int64
	Allocated bool
	IsDir     bool
	SI_Times  *TimeStamps

	// If multiple filenames are given, we list them here.
	Filenames []*FilenameInfo

	Attributes []*Attribute
}

func ModelMFTEntry(mft_entry *MFT_ENTRY) (*NTFSFileInformation, error) {
	full_path, _ := GetFullPath(mft_entry)
	mft_id := mft_entry.Get("record_number").AsInteger()

	result := &NTFSFileInformation{
		FullPath:  full_path,
		MFTID:     mft_id,
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
		if attr.Get("type").AsInteger() == 128 && result.Size == 0 {
			result.Size = attr.Size()
		}

		attr_type := attr.Get("type")
		attr_id := attr.Get("attribute_id").AsInteger()
		result.Attributes = append(result.Attributes, &Attribute{
			Type:   attr_type.AsString(),
			TypeId: attr_type.AsInteger(),
			Inode: fmt.Sprintf("%v-%v-%v",
				mft_id, attr_type.AsInteger(), attr_id),
			Size: attr.Size(),
			Id:   attr_id,
			Name: attr.Name(),
		})
	}

	return result, nil
}
