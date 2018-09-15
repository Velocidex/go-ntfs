package ntfs

const NTFS_PROFILE = `
{
   "NTFS_BOOT_SECTOR": [512, {
       "oemname": [3, ["String", {"length": 8}]],
       "sector_size": [11, ["unsigned short"]],
       "_cluster_size": [13, ["unsigned char"]],
        "_volume_size":   [40, ["unsigned long"]],
        "_mft_cluster":   [48, ["unsigned long"]],
        "_mirror_mft_cluster":   [56, ["unsigned long"]],
        "_mft_record_size": [64, ["char"]],
        "index_record_size": [68, ["unsigned char"]],
       "serial": [72, ["String", {"length": 8}]],
        "magic": [510, ["unsigned short"]]
    }],

    "MFT_ENTRY": [0, {
        "magic": [0, ["String", {"length": 4}]],
        "fixup_offset": [4, ["unsigned short"]],
        "fixup_count":  [6, ["unsigned short"]],
        "logfile_sequence_number": [8, ["unsigned long long"]],
        "sequence_value": [16, ["unsigned short"]],
        "link_count": [18, ["unsigned short"]],
        "attribute_offset": [20, ["unsigned short"]],
        "flags": [22, ["Flags", {
            "target": "unsigned short",
            "bitmap": {
                "ALLOCATED": 0,
                "DIRECTORY": 1
            }
        }]],
        "mft_entry_size": [24, ["unsigned short"]],
        "mft_entry_allocated": [28, ["unsigned short"]],
        "base_record_reference": [32, ["unsigned long long"]],
        "next_attribute_id": [40, ["unsigned short"]],
        "record_number": [44, ["unsigned long"]]
    }],

    "NTFS_ATTRIBUTE": [0, {
        "type": [0, ["Enumeration", {
            "target": "unsigned int",
            "choices": {
                "16": "$STANDARD_INFORMATION",
                "32": "$ATTRIBUTE_LIST",
                "48": "$FILE_NAME",
                "64": "$OBJECT_ID",
                "80": "$SECURITY_DESCRIPTOR",
                "96": "$VOLUME_NAME",
                "112": "$VOLUME_INFORMATION",
                "128": "$DATA",
                "144": "$INDEX_ROOT",
                "160": "$INDEX_ALLOCATION",
                "176": "$BITMAP",
                "192": "$REPARSE_POINT",
                "256": "$LOGGED_UTILITY_STREAM"
            }
        }]],
        "length": [4, ["unsigned int"]],
        "resident": [8, ["Enumeration", {
            "target":"unsigned char",
            "choices": {
                "0": "RESIDENT",
                "1": "NON-RESIDENT"
           }
        }]],

        "name_length": [9, ["unsigned char"]],
        "name_offset": [10, ["unsigned short"]],
        "flags": [12, ["Flags", {
            "target": "unsigned short",
            "maskmap": {
                "COMPRESSED" : 1,
                "ENCRYPTED": 16384,
                "SPARSE": 32768
            }
        }]],
        "attribute_id": [14, ["unsigned short"]],

        "content_size": [16, ["unsigned int"]],
        "content_offset": [20, ["unsigned short"]],
        "runlist_vcn_start": [16, ["unsigned long long"]],
        "runlist_vcn_end": [24, ["unsigned long long"]],
        "runlist_offset": [32, ["unsigned short"]],
        "compression_unit_size": [34, ["unsigned short"]],
        "allocated_size": [40, ["unsigned long long"]],
        "actual_size": [48, ["unsigned long long"]],
        "initialized_size": [56, ["unsigned long long"]]
    }],

    "STANDARD_INFORMATION": [0, {
        "create_time": [0, ["WinFileTime"]],
        "file_altered_time": [8, ["WinFileTime"]],
        "mft_altered_time": [16, ["WinFileTime"]],
        "file_accessed_time": [24, ["WinFileTime"]],
        "flags": [32, ["Flags", {
            "target": "unsigned int",
            "maskmap": {
                "READ_ONLY": 1,
                "HIDDEN": 2,
                "SYSTEM": 4,
                "ARCHIVE":32,
                "DEVICE":64,
                "NORMAL":128,
                "TEMPORARY":256,
                "SPARSE":512,
                "REPARSE_POINT":1024,
                "COMPRESSED":2048,
                "OFFLINE":4096,
                "NOT_INDEXED":8192,
                "ENCRYPTED":16384
            }
        }]],
        "max_versions": [36, ["unsigned int"]],
        "version": [40, ["unsigned int"]],
        "class_id": [44, ["unsigned int"]],
        "owner_id": [48, ["unsigned int"]],
        "sid": [52, ["unsigned int"]],
        "quota": [56, ["unsigned long long"]],
        "usn": [64, ["unsigned int"]]
    }],

    "FILE_NAME": [0, {
        "mftReference": [0, ["BitField", {
            "target": "unsigned long long",
            "start_bit": 0,
            "end_bit": 48
        }]],
        "seq_num": [6, ["short int"]],
        "created": [8, ["WinFileTime"]],
        "file_modified": [16, ["WinFileTime"]],
        "mft_modified": [24, ["WinFileTime"]],
        "file_accessed": [32, ["WinFileTime"]],
        "allocated_size": [40, ["unsigned long long"]],
        "size": [48, ["unsigned long long"]],
        "flags": [56, ["Flags", {
            "target":"unsigned int",
            "bitmap": {
                "READ_ONLY": 1,
                "HIDDEN": 2,
                "SYSTEM": 4,
                "ARCHIVE":32,
                "DEVICE":64,
                "NORMAL":128,
                "TEMPORARY":256,
                "SPARSE":512,
                "REPARSE_POINT":1024,
                "COMPRESSED":2048,
                "OFFLINE":4096,
                "NOT_INDEXED":8192,
                "ENCRYPTED":16384
            }}]],
        "reparse_value": [60, ["unsigned int"]],
        "_length_of_name": [64, ["byte"]],
        "name_type": [65, ["Enumeration", {
            "target": "byte",
            "choices": {
                "0": "POSIX",
                "1": "Win32",
                "2": "DOS",
                "3": "DOS+Win32"
            }
        }]],
        "name": [66, ["char"]]
    }],

    "STANDARD_INDEX_HEADER": [42, {
        "magicNumber": [0, ["Signature", {
            "value": "INDX"
        }]],

        "fixup_offset": [4, ["unsigned short"]],
        "fixup_count": [6, ["unsigned short"]],
        "logFileSeqNum": [8, ["unsigned long long"]],
        "vcnOfINDX": [16, ["unsigned long long"]],
        "node": [24, ["INDEX_NODE_HEADER"]]
    }],

    "INDEX_RECORD_ENTRY": [0, {
        "mftReference": [0, ["BitField", {
            "target": "unsigned long long",
            "start_bit": 0,
            "end_bit":48
        }]],
        "seq_num": [6, ["short int"]],
        "sizeOfIndexEntry": [8, ["unsigned short"]],
        "filenameOffset": [10, ["unsigned short"]],
        "flags": [12, ["unsigned int"]],
        "file": [16, ["FILE_NAME"]]
    }],

    "INDEX_ROOT": [0, {
        "type": [0, ["unsigned int"]],
        "collation_rule": [4, ["unsigned int"]],
        "idxalloc_size_b": [8, ["unsigned int"]],
        "idx_size_c": [12, ["unsigned int"]],
        "node": [16, ["INDEX_NODE_HEADER"]]
    }],

    "INDEX_NODE_HEADER": [16, {
        "offset_to_index_entry": [0, ["unsigned int"]],
        "offset_to_end_index_entry": [4, ["unsigned int"]]
    }],

    "ATTRIBUTE_LIST_ENTRY": [0, {
        "type": [0, ["unsigned int"]],
        "length": [4, ["unsigned short int"]],
        "name_length": [6, ["byte"]],
        "offset_to_name": [7, ["byte"]],
        "starting_vcn": [8, ["unsigned long long"]],
        "mftReference": [16, ["BitField", {
            "target": "unsigned long long",
            "start_bit": 0,
            "end_bit": 48
        }]],
        "attribute_id": [24, ["byte"]]
    }]
}
`
