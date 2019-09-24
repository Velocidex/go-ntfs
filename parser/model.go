package parser

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
	TypeId uint64
	Id     uint64
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

func ModelMFTEntry(ntfs *NTFSContext, mft_entry *MFT_ENTRY) (*NTFSFileInformation, error) {
	full_path, _ := GetFullPath(ntfs, mft_entry)
	mft_id := mft_entry.Record_number()

	result := &NTFSFileInformation{
		FullPath:  full_path,
		MFTID:     int64(mft_id),
		Allocated: mft_entry.Flags().IsSet("ALLOCATED"),
		IsDir:     mft_entry.Flags().IsSet("DIRECTORY"),
	}

	si, err := mft_entry.StandardInformation(ntfs)
	if err == nil {
		result.SI_Times = &TimeStamps{
			CreateTime:       si.Create_time().Time,
			FileModifiedTime: si.File_altered_time().Time,
			MFTModifiedTime:  si.Mft_altered_time().Time,
			AccessedTime:     si.File_accessed_time().Time,
		}
	}

	for _, filename := range mft_entry.FileName(ntfs) {
		result.Filenames = append(result.Filenames, &FilenameInfo{
			Times: TimeStamps{
				CreateTime:       filename.Created().Time,
				FileModifiedTime: filename.File_modified().Time,
				MFTModifiedTime:  filename.Mft_modified().Time,
				AccessedTime:     filename.File_accessed().Time,
			},
			Type: filename.NameType().Name,
			Name: filename.Name(),
		})
	}

	for _, attr := range mft_entry.EnumerateAttributes(ntfs) {
		attr_type := attr.Type()
		attr_id := attr.Attribute_id()

		if attr_type.Name == "$DATA" && result.Size == 0 {
			result.Size = attr.DataSize()
		}

		result.Attributes = append(result.Attributes, &Attribute{
			Type:   attr_type.Name,
			TypeId: attr_type.Value,
			Inode: fmt.Sprintf("%v-%v-%v",
				mft_id, attr_type.Value, attr_id),
			Size: attr.DataSize(),
			Id:   uint64(attr_id),
			Name: attr.Name(),
		})
	}

	return result, nil
}
