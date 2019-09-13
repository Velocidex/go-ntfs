package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

func (self *NTFS_BOOT_SECTOR) ClusterSize() int64 {
	return int64(self._cluster_size()) * int64(self.Sector_size())
}

func (self *NTFS_BOOT_SECTOR) BlockCount() int64 {
	return int64(self._volume_size()) / int64(self.ClusterSize())
}

func (self *NTFS_BOOT_SECTOR) RecordSize() int64 {
	_record_size := int64(self._mft_record_size())
	if _record_size > 0 {
		return _record_size * self.ClusterSize()
	}
	return 1 << uint32(-_record_size)
}

// The MFT entry needs to be fixed up. This method extracts the
// MFT_ENTRY from disk into a buffer and perfoms the fixups. We then
// return an MFT_ENTRY instantiated over this fixed up buffer.
func FixUpDiskMFTEntry(mft *MFT_ENTRY) (io.ReaderAt, error) {
	fixup_offset := mft.Offset + int64(mft.Fixup_offset())
	fixup_magic := make([]byte, 2)
	_, err := mft.Reader.ReadAt(fixup_magic, fixup_offset)
	if err != nil {
		return nil, err
	}

	fixup_offset += 2

	// Read the fixup table
	fixup_table := [][]byte{}
	for i := int64(0); i < int64(mft.Fixup_count())-1; i++ {
		table_item := make([]byte, 2)
		_, err := mft.Reader.ReadAt(table_item, fixup_offset+2*i)
		if err != nil {
			return nil, err
		}
		fixup_table = append(fixup_table, table_item)
	}

	// Read the entire MFT entry into the buffer and then apply
	// the fixup table.
	buffer := make([]byte, mft.Mft_entry_allocated())
	_, err = mft.Reader.ReadAt(buffer, mft.Offset)
	if err != nil {
		return nil, err
	}
	for idx, fixup_value := range fixup_table {
		fixup_offset := (idx+1)*512 - 2
		if buffer[fixup_offset] != fixup_magic[0] ||
			buffer[fixup_offset+1] != fixup_magic[1] {
			return nil, errors.New(fmt.Sprintf("Fixup error with MFT %d",
				mft.Record_number()))
		}

		// Apply the fixup
		buffer[fixup_offset] = fixup_value[0]
		buffer[fixup_offset+1] = fixup_value[1]
	}

	return bytes.NewReader(buffer), nil
}

// Find the root MFT_ENTRY object. Returns a reader over the $MFT file.
func BootstrapMFT(ntfs *NTFSContext) (io.ReaderAt, error) {
	// The MFT is a table of MFT_ENTRY records read from an
	// abstracted reader which is itself a $DATA attribute of the
	// first MFT record:

	// MFT[0] -> Attr $DATA contains the entire $MFT stream.

	// We therefore need to bootstrap the MFT:
	// 1. Read the first entry in the first cluster using the disk reader.
	// 2. Search for the $DATA attribute.
	// 3. Reconstruct the runlist and RunReader from this attribute.
	// 4. Instantiate the MFT over this new reader.
	offset := int64(ntfs.Boot._mft_cluster()) * ntfs.Boot.ClusterSize()
	disk_mft := ntfs.Profile.MFT_ENTRY(ntfs.DiskReader, offset)

	fixed_up_reader, err := FixUpDiskMFTEntry(disk_mft)
	if err != nil {
		return nil, err
	}

	root_mft := ntfs.Profile.MFT_ENTRY(fixed_up_reader, 0)

	// Find the $DATA attribute of the root entry. This will
	// contain the full $MFT file.
	for _, attr := range root_mft.EnumerateAttributes(ntfs) {
		if attr.Type().Name == "$DATA" {
			return attr.Data(ntfs), nil
		}
	}

	return nil, errors.New("$DATA attribute not found for $MFT")
}

func (self *NTFS_BOOT_SECTOR) IsValid() error {
	if self.Magic() != 0xaa55 {
		return errors.New("Invalid magic")
	}

	switch self.ClusterSize() {
	case 0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40,
		0x80, 0x100, 0x200, 0x400, 0x800, 0x1000:
		break
	default:
		return errors.New(
			fmt.Sprintf("Invalid cluster size %x",
				self.ClusterSize()))
	}

	sector_size := self.Sector_size()
	if sector_size == 0 || (sector_size%512 != 0) {
		return errors.New("Invalid sector_size")
	}

	if self.BlockCount() == 0 {
		return errors.New("Volume size is 0")
	}

	return nil
}
