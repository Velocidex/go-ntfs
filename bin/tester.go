package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"sync"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	test_command = app.Command(
		"test", "Test file reading by comparing with TSK.")

	test_command_file_arg = test_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	test_command_icat_path = test_command.Flag(
		"icat_path", "Path to the icat binary").
		Default("icat").String()

	test_command_start_id = test_command.Flag(
		"start", "First MFT ID to test").Default("0").Int64()

	test_command_max_size = test_command.Flag(
		"max_size", "Skip testing files of this size").
		Default("104857600").Int64()
)

func calcHashWithTSK(inode string) (string, int64, error) {
	command := exec.Command(*test_command_icat_path,
		(*test_command_file_arg).Name(), inode)
	stdout_pipe, err := command.StdoutPipe()
	if err != nil {
		return "", 0, err
	}

	err = command.Start()
	if err != nil {
		return "", 0, err
	}

	wg := &sync.WaitGroup{}
	md5_sum := md5.New()
	offset := 0

	wg.Add(1)
	go func() {
		defer wg.Done()

		buff := make([]byte, 1000000)
		for {
			n, err := stdout_pipe.Read(buff)
			if err != nil && err != io.EOF {
				return
			}

			if n == 0 {
				return
			}

			md5_sum.Write(buff[:n])
			offset += n
		}
	}()

	wg.Wait()

	return hex.EncodeToString(md5_sum.Sum(nil)), int64(offset), nil
}

func doTest() {
	reader, err := parser.NewPagedReader(*test_command_file_arg, 1024, 10000)
	kingpin.FatalIfError(err, "Can not open MFT file")

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	mft_stream, err := parser.GetDataForPath(ntfs_ctx, "$MFT")
	kingpin.FatalIfError(err, "Can not open filesystem")

	buffer := make([]byte, 1024*1024)

	for i := int64(*test_command_start_id); i < parser.RangeSize(mft_stream)/ntfs_ctx.RecordSize; i++ {
		mft_entry, err := ntfs_ctx.GetMFT(i)
		if err != nil {
			continue
		}

		if !mft_entry.Flags().IsSet("ALLOCATED") {
			continue
		}

		// Skip MFT entries which belong to an attribute list.
		if mft_entry.Base_record_reference() != 0 {
			continue
		}

		for _, attr := range mft_entry.EnumerateAttributes(ntfs_ctx) {
			reader, err := parser.OpenStream(ntfs_ctx,
				mft_entry, attr.Type().Value, attr.Attribute_id())
			if err != nil {
				continue
			}

			size := parser.RangeSize(reader)
			if size > *test_command_max_size {
				continue
			}

			inode := fmt.Sprintf("%v-%v-%v",
				i, attr.Type().Value, attr.Attribute_id())

			fmt.Printf("%s :", inode)
			md5_sum := md5.New()
			offset := int64(0)

			for {
				n, err := reader.ReadAt(buffer, offset)
				if n == 0 || err == io.EOF {
					break
				}

				data := buffer[:n]
				md5_sum.Write(data)
				offset += int64(n)
			}

			our_hash := hex.EncodeToString(md5_sum.Sum(nil))
			fmt.Printf("%v %v\n", our_hash, offset)

			// Shell out to icat to calculate the hash
			tsk_hash, tsk_length, err := calcHashWithTSK(inode)
			kingpin.FatalIfError(err, "Can not shell to tsk")

			fmt.Printf("   TSK hash: %v %v\n", tsk_hash, tsk_length)

			// TSK makes up its own ntfs stream IDs for
			// some streams. This means when we try to
			// open the stream with the original id, icat
			// can not find it. We skip those cases
			// because it is very hard to tell the id that
			// TSK picks (it is random and not related to
			// the actual stream ID).
			if tsk_length == 0 {
				continue
			}

			if tsk_length != offset || tsk_hash != our_hash {
				fmt.Printf("******* Error!\n")
			}

		}
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case test_command.FullCommand():
			doTest()
		default:
			return false
		}
		return true
	})
}

type readAdapter struct {
	pos    int64
	reader io.ReaderAt
}

func (self *readAdapter) Read(buf []byte) (res int, err error) {
	res, err = self.reader.ReadAt(buf, self.pos)
	self.pos += int64(res)

	return res, err
}
