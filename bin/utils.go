package main

import (
	"strings"

	"www.velocidex.com/golang/go-ntfs/parser"
)

func getADSName(filename string) string {
	parts := strings.SplitN(filename, ":", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func GetMFTEntry(ntfs_ctx *parser.NTFSContext, filename string) (*parser.MFT_ENTRY, error) {
	mft_idx, _, _, err := parser.ParseMFTId(filename)
	if err == nil {
		// Access by mft id (e.g. 1234-128-6)
		return ntfs_ctx.GetMFT(mft_idx)
	} else {
		// Access by filename.
		dir, err := ntfs_ctx.GetMFT(5)
		if err != nil {
			return nil, err
		}

		return dir.Open(ntfs_ctx, filename)
	}
}
