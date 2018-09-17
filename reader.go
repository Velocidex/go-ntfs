// This reader is needed for reading raw windows devices, such as
// \\.\c: On windows, such devices may only be read using sector
// alignment in whole sector numbers. This reader implements page
// aligned reading and adds pages to an LRU cache to make accessing
// various field members faster.

package ntfs

import (
	"io"

	lru "github.com/hashicorp/golang-lru"
)

type PagedReader struct {
	reader   io.ReaderAt
	pagesize int64
	lru      *lru.Cache
}

func (self *PagedReader) ReadAt(buf []byte, offset int64) (int, error) {
	buf_idx := 0
	for {
		// How much is left in this page to read?
		to_read := int(self.pagesize - offset%self.pagesize)

		// How much do we need to read into the buffer?
		if to_read > len(buf)-buf_idx {
			to_read = len(buf) - buf_idx
		}

		// Are we done?
		if to_read == 0 {
			return buf_idx, nil
		}

		var page_buf []byte

		page := offset - offset%self.pagesize
		cached_page_buf, pres := self.lru.Get(page)
		if !pres {
			// Read this page into memory.
			page_buf = make([]byte, self.pagesize)
			_, err := self.reader.ReadAt(page_buf, page)
			if err != nil {
				return buf_idx, err
			}

			self.lru.Add(page, page_buf)
		} else {
			page_buf = cached_page_buf.([]byte)
		}

		for i := 0; i < to_read; i++ {
			buf[buf_idx+i] = page_buf[i+int(offset%self.pagesize)]
		}

		offset += int64(to_read)
		buf_idx += to_read
	}
}

func NewPagedReader(reader io.ReaderAt) (*PagedReader, error) {
	cache, err := lru.New(50)
	if err != nil {
		return nil, err
	}

	return &PagedReader{
		reader:   reader,
		pagesize: 1024,
		lru:      cache,
	}, nil
}
