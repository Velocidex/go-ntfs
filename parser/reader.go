package parser

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// Keep pages in a free list to avoid allocations.
type FreeList struct {
	mu       sync.Mutex
	pagesize int64

	freelist sync.Pool
}

func (self *FreeList) Get() []byte {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.freelist.Get().([]byte)
}

func (self *FreeList) Put(in []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.freelist.Put(in)
}

// This reader is needed for reading raw windows devices, such as
// \\.\c: On windows, such devices may only be read using sector
// alignment in whole sector numbers. This reader implements page
// aligned reading and adds pages to an LRU cache to make accessing
// various field members faster.
type PagedReader struct {
	mu sync.Mutex

	reader   io.ReaderAt
	pagesize int64
	lru      *LRU
	eofPos   int64

	freelist *FreeList

	Hits int64
	Miss int64
}

func (self *PagedReader) IsFixed(offset int64) bool {
	return false
}

func (self *PagedReader) VtoP(offset int64) int64 {
	return offset
}

// ReadAt reads a buffer from an offset in the backing file.
//
// The following semantics are used:
//  1. Reading within the file will always fill the buffer completely
//     with n = len(buf)
//  2. Reading a buffer that starts within the file and ends past the
//     file will also return a full buffer with n = len(buf) but will
//     indicate the read went past the end of the buffer with err = EOF
//  3. Reading outside the bounds of the file will return n = 0 and
//     err = EOF
//
// Readers generally need to find the exact size of the file some
// other way - reading the file sequentially in blocks will result in
// over-reading and the last block being padded up to blocksize.
//
// For example, On windows the size of a block device can not be found
// with os.Lstat but using WMI.
func (self *PagedReader) ReadAt(buf []byte, offset int64) (res int, ret_err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// If the read is very large and a multiple of pagesize, it is
	// faster to just delegate reading to the underlying reader.
	if len(buf) > 10*int(self.pagesize) && len(buf)%int(self.pagesize) == 0 {
		return self.reader.ReadAt(buf, offset)
	}

	buf_idx := 0
	for {
		// How much is left in this page to read?
		to_read := int(self.pagesize - offset%self.pagesize)

		// How much do we need to read into the buffer?
		if to_read > len(buf)-buf_idx {
			to_read = len(buf) - buf_idx
		}

		// If we hit EOF, cap the read to the number of bytes left
		// in the file.
		if self.eofPos != -1 && offset+int64(to_read) > self.eofPos {
			to_read = int(self.eofPos - offset)
		}

		// Are we done?
		if to_read == 0 {
			return buf_idx, ret_err
		}

		var page_buf []byte

		page := offset - offset%self.pagesize
		cached_page_buf, pres := self.lru.Get(int(page))
		if !pres {
			self.Miss += 1
			DebugPrint("Cache miss for %x (%x) (%d)\n", page, self.pagesize,
				self.lru.Len())

			// Read this page into memory - already holding the lock.
			page_buf = self.freelist.Get()
			n, err := self.reader.ReadAt(page_buf, page)
			if err != nil && err != io.EOF {
				// We wont be putting the page in the LRU, just return
				// it to the freelist.
				self.freelist.Put(page_buf)
				return buf_idx, err
			}

			// Clear the rest of the page because it is going to the
			// lru.
			for i := n; i < int(self.pagesize); i++ {
				page_buf[i] = 0
			}

			self.lru.Add(int(page), page_buf)

			// The entire read range is outside the bounds of the
			// file, just fail the read with EOF. For read ranges
			// which are partially inside the file, they will be
			// padded and EOF will be emitted.
			if errors.Is(err, io.EOF) {
				if n == 0 && buf_idx == 0 {
					return 0, err
				}
				ret_err = err
			}

		} else {
			self.Hits += 1
			page_buf = cached_page_buf.([]byte)
		}

		// Copy the relevant data from the page.
		page_offset := int(offset % self.pagesize)
		copy(buf[buf_idx:buf_idx+to_read],
			page_buf[page_offset:page_offset+to_read])

		offset += int64(to_read)
		buf_idx += to_read
		if debug && (self.Hits+self.Miss)%10000 == 0 {
			fmt.Printf("PageCache hit %v miss %v (%v)\n", self.Hits, self.Miss,
				float64(self.Hits)/float64(self.Miss))
		}
	}
}

func (self *PagedReader) Flush() {
	self.lru.Purge()

	flusher, ok := self.reader.(Flusher)
	if ok {
		flusher.Flush()
	}
}

func NewPagedReader(reader io.ReaderAt, pagesize int64, cache_size int) (*PagedReader, error) {
	DebugPrint("Creating cache of size %v\n", cache_size)

	self := &PagedReader{
		reader:   reader,
		pagesize: pagesize,
		freelist: &FreeList{
			pagesize: pagesize,
			freelist: sync.Pool{
				New: func() interface{} {
					return make([]byte, pagesize)
				},
			},
		},
		eofPos: -1,
	}

	// By default 10mb cache.
	cache, err := NewLRU(cache_size, func(key int, value interface{}) {
		// Put the page back on the free list
		self.freelist.Put(value.([]byte))
	}, "NewPagedReader")
	if err != nil {
		return nil, err
	}

	self.lru = cache

	return self, nil
}

// Invalidate the disk cache
type Flusher interface {
	Flush()
}
