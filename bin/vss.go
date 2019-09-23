package main

import (
	"fmt"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	vss_command = app.Command(
		"vss", "Inspect VSS.")

	vss_command_file_arg = vss_command.Arg(
		"file", "The image file to inspect",
	).Required().OpenFile(os.O_RDONLY, os.FileMode(0666))
)

func doVSS() {
	reader, _ := parser.NewPagedReader(*vss_command_file_arg, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	vss_header := ntfs_ctx.Profile.VSS_VOLUME_HEADER(reader, 0x1e00)
	fmt.Printf("%v", vss_header.DebugString())

	for CatalogOffset := vss_header.CatalogOffset(); CatalogOffset > 0; {
		catalog_header := ntfs_ctx.Profile.VSS_CATALOG_HEADER(
			reader, CatalogOffset)

		fmt.Printf("%v", catalog_header.DebugString())
		CatalogOffset = catalog_header.NextOffset()

		offset := int64(catalog_header.Offset) + int64(catalog_header.Size())
		end := int64(catalog_header.Offset) + 0x00004000

		for offset < end {
			entry1 := ntfs_ctx.Profile.VSS_CATALOG_ENTRY_1(reader, offset)
			switch entry1.EntryType() {
			case 2:
				entry2 := ntfs_ctx.Profile.VSS_CATALOG_ENTRY_2(reader, offset)
				fmt.Printf("%v", entry2.DebugString())

				store_guid_filename := fmt.Sprintf(
					"System Volume Information/%s%s",
					entry2.StoreGUID().AsString(),
					catalog_header.Identifier().AsString())

				fmt.Printf("Store is %s\n", store_guid_filename)
				printStore(ntfs_ctx, store_guid_filename)

				offset += int64(entry2.Size())
			case 3:
				entry3 := ntfs_ctx.Profile.VSS_CATALOG_ENTRY_3(reader, offset)
				fmt.Printf("%v", entry3.DebugString())
				offset += int64(entry3.Size())

			default:
				fmt.Printf("%v", entry1.DebugString())
				offset += int64(entry1.Size())
			}
		}

	}
}

func printStore(ntfs_ctx *parser.NTFSContext, store_guid_filename string) {
	data_stream, err := parser.GetDataForPath(ntfs_ctx, store_guid_filename)
	kingpin.FatalIfError(err, "Can not open store")

	vss_store_block_header := ntfs_ctx.Profile.VSS_STORE_BLOCK_HEADER(
		data_stream, 0)

	fmt.Printf("%v", vss_store_block_header.DebugString())

	vss_store_info := ntfs_ctx.Profile.VSS_STORE_INFORMATION(
		data_stream, int64(vss_store_block_header.Size()))

	fmt.Printf("%v", vss_store_info.DebugString())
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "vss":
			doVSS()
		default:
			return false
		}
		return true
	})
}
