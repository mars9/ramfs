package ramfs

import "io"

type perror string

func (e perror) Error() string { return string(e) }

type buffer interface {
	ReadAt(p []byte, offset int64) (int, error)
	WriteAt(p []byte, offset int64) (int, error)
	Len() uint64
	Close() error
}

type file struct {
	size      uint64
	block     map[uint64][]byte
	blockSize uint64
}

func newFile(blockSize uint64) *file {
	return &file{
		block:     make(map[uint64][]byte),
		blockSize: blockSize,
	}
}

func (f *file) WriteAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, perror("negative offset")
	}

	off := uint64(offset)
	if off > f.size {
		off = f.size
	}
	num := off / f.blockSize
	off = off % f.blockSize
	expanded := false

	n := 0
	for len(p) > 0 {
		consume := f.blockSize - off
		if consume > uint64(len(p)) {
			consume = uint64(len(p))
		}

		if _, found := f.block[num]; !found {
			f.block[num] = make([]byte, consume)
			expanded = true
		} else {
			if (off + consume) > uint64(len(f.block[num])) {
				data := make([]byte, off+consume)
				copy(data, f.block[num])
				f.block[num] = data
				expanded = true
			}
		}

		m := copy(f.block[num][off:], p)
		p = p[m:]
		n += m

		if expanded {
			if uint64(m) > f.size {
				f.size += uint64(m) - f.size
			} else {
				f.size += uint64(m)
			}
		}

		off = 0
		num++
	}
	return n, nil
}

func (f *file) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, perror("negative offset")
	}
	off := uint64(offset)

	if off > f.size {
		return 0, io.EOF
	}
	num := off / f.blockSize

	count := uint64(len(p))
	if off+count > f.size {
		count = f.size - off
	}
	off = off % f.blockSize

	n := 0
	for p = p[0:count]; len(p) > 0 && len(f.block[num][off:]) > 0; {
		m := copy(p, f.block[num][off:])
		p = p[m:]
		n += m
		off = 0
		num++
	}
	return n, nil
}

func (f *file) Len() uint64  { return f.size }
func (f *file) Close() error { return nil }
