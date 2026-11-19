package pkginst

import "io"

type sliceReader []byte

func (s sliceReader) ReadAt(b []byte, off int64) (int, error) {
	if off >= int64(len(s)) {
		return 0, io.EOF
	}
	n := copy(b, s[off:])
	return n, nil
}
