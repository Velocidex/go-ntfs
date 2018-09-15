package ntfs

import (
	"fmt"
	"io"
	"time"

	"www.velocidex.com/golang/vtypes"
)

type WinFileTimeParser struct {
	*vtypes.IntParser
}

func (self WinFileTimeParser) Copy() vtypes.Parser {
	return &WinFileTimeParser{
		IntParser: self.IntParser.Copy().(*vtypes.IntParser),
	}
}

func (self *WinFileTimeParser) AsDate(offset int64, reader io.ReaderAt) time.Time {
	return time.Unix(self.AsInteger(offset, reader)/10000000-11644473600, 0)
}

func (self *WinFileTimeParser) AsString(offset int64, reader io.ReaderAt) string {
	return self.AsDate(offset, reader).String()
}

func (self *WinFileTimeParser) DebugString(offset int64, reader io.ReaderAt) string {
	return fmt.Sprintf("[%s] %#0x (%s)", self.Name, self.AsInteger(offset, reader),
		self.AsString(offset, reader))
}

func AddParsers(profile *vtypes.Profile) {
	int_parser, pres := profile.GetParser("uint64")
	if pres {
		parser := &WinFileTimeParser{
			int_parser.Copy().(*vtypes.IntParser),
		}
		parser.SetName("WinFileTime")
		profile.AddParser("WinFileTime", parser)
	}
}
