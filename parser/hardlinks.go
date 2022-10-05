/* This code traverse MFT entries to discover all the paths an entry
   is known by.

   In NTFS A file (MFT Entry) may exist in multiple direcotries this
   is called hardlinks.

   You can create a hardlink using the fsutils utility:

   C:> fsutil.exe hardlink create C:/users/test/X.txt "C:/users/test/downloads/X.txt"
   Hardlink created for C:\users\test\X.txt <<===>> C:\users\test\downloads\X.txt

   This adds a $FILE_NAME attribute to the MFT entry and points it at
   a different parent.
*/

package parser

import (
	"fmt"
)

type Visitor struct {
	Paths [][]string
	Max   int
}

func (self *Visitor) Add(idx int, depth int) int {
	self.Paths = append(self.Paths, CopySlice(self.Paths[idx][:depth]))
	return len(self.Paths) - 1
}

func (self *Visitor) AddComponent(idx int, component string) {
	self.Paths[idx] = append(self.Paths[idx], component)
}

func (self *Visitor) Components() [][]string {
	for _, p := range self.Paths {
		ReverseStringSlice(p)
	}
	return self.Paths
}

// Walks the MFT entry to get all file names to this MFT entry.
func GetHardLinks(ntfs *NTFSContext, mft_id uint64, max int) [][]string {
	visitor := &Visitor{
		Paths: [][]string{[]string{}},
	}

	mft_entry_summary, err := ntfs.GetMFTSummary(mft_id)
	if err != nil {
		return nil
	}
	getNames(ntfs, mft_entry_summary, visitor, 0, 0)

	return visitor.Components()
}

func getNames(ntfs *NTFSContext,
	mft_entry *MFTEntrySummary, visitor *Visitor, idx, depth int) {

	if depth > ntfs.MaxDirectoryDepth {
		visitor.AddComponent(idx, "<DirTooDeep>")
		visitor.AddComponent(idx, "<Err>")
		return
	}

	filenames := []*FNSummary{}
	for _, fn := range mft_entry.Filenames {
		switch fn.NameType {
		case "Win32", "DOS+Win32", "POSIX":
			filenames = append(filenames, &fn)
		}
	}

	for i, fn := range filenames {
		// The first FN entry continues to visit the same path but the
		// next one will add a new path.
		visitor_idx := idx
		if i > 0 {
			visitor_idx = visitor.Add(idx, depth)
			if visitor_idx > visitor.Max {
				continue
			}
		}

		visitor.AddComponent(visitor_idx, fn.Name)

		if fn.ParentEntryNumber == 5 || fn.ParentEntryNumber == 0 {
			continue
		}

		parent_entry, err := ntfs.GetMFTSummary(fn.ParentEntryNumber)
		if err != nil {
			visitor.AddComponent(visitor_idx, err.Error())
			visitor.AddComponent(visitor_idx, "<Err>")
			continue
		}

		if fn.ParentSequenceNumber != parent_entry.Sequence {
			visitor.AddComponent(visitor_idx,
				fmt.Sprintf("<Parent %v-%v need %v>", fn.ParentEntryNumber,
					parent_entry.Sequence, fn.ParentSequenceNumber))
			visitor.AddComponent(visitor_idx, "<Err>")
			continue
		}

		getNames(ntfs, parent_entry, visitor, visitor_idx, depth+1)
	}
}
