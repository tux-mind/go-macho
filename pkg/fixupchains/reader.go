package fixupchains

import (
	"bytes"
	"fmt"
	"io"
)

type LazyRebasedReader struct {
	// lazy loaded fields
	dcf          *DyldChainedFixups
	baseAddr     uint64
	rebases      map[uint64]Rebase
	pointerSize  uint64
	readPointer  func(src []byte) uint64
	writePointer func(dst []byte, ptr uint64)

	// required fields

	// a function that returns a fully parsed DyldChainedFixups (e.g. DyldChainedFixups.Parse() )
	GetDyldchainFixups func() (*DyldChainedFixups, error)
	// a function that returns the preffered load address
	GetBaseAddr func() uint64
	// the reader to patch
	Reader io.ReaderAt
}

func (lrr *LazyRebasedReader) init() (err error) {
	if lrr.dcf, err = lrr.GetDyldchainFixups(); err != nil {
		return fmt.Errorf("cannot retrieve fixups: %v", err)
	} else if _, err = lrr.dcf.Parse(); err != nil {
		return fmt.Errorf("cannot parse fixups: %v", err)
	}

	lrr.rebases = make(map[uint64]Rebase)
	lrr.baseAddr = lrr.GetBaseAddr()

	for _, s := range lrr.dcf.Starts {
		if lrr.pointerSize == 0 && s.PageCount > 0 {
			if lrr.pointerSize, err = ptrSize(s.PointerFormat); err != nil {
				return err
			}
		}
		for _, f := range s.Fixups {
			if r, ok := f.(Rebase); ok {
				lrr.rebases[r.Offset()] = r
			}
		}
	}

	bo := lrr.dcf.bo

	switch lrr.pointerSize {
	case 8:
		lrr.readPointer = bo.Uint64
		lrr.writePointer = bo.PutUint64
	case 4:
		lrr.readPointer = func(x []byte) uint64 { return uint64(bo.Uint32(x)) }
		lrr.writePointer = func(x []byte, y uint64) { bo.PutUint32(x, uint32(y)) }
	case 0:
		// no fixups, patchBytes will have nothing to work on
		break
	default:
		return fmt.Errorf("unexpected pointer size: %d", lrr.pointerSize)
	}

	return nil
}

func (lrr *LazyRebasedReader) ReadAt(p []byte, off int64) (n int, err error) {
	if lrr.dcf == nil {
		if err = lrr.init(); err != nil {
			return 0, fmt.Errorf("failed to initialise rebased reader: %v", err)
		}
	}

	if n, err = lrr.Reader.ReadAt(p, off); err != nil {
		return n, err
	}

	if err = lrr.patchReadBytes(p, uint64(off)); err != nil {
		return 0, err
	}
	return n, err
}

func (lrr *LazyRebasedReader) patchReadBytes(p []byte, off uint64) error {
	// TODO: implement a quick check that returns nil when (off, off+len(p)) is outside the fixed up pages.
	//     : I can't answer the question "can a chain overflow its page?": if so, this cehck is not possible.
	//     : An alternative would be to store each chain start and end location when we walk them.

	max := off + uint64(len(p))
	buf := make([]byte, lrr.pointerSize)

	for rOff, r := range lrr.rebases {
		if rOff+lrr.pointerSize < off || rOff > max {
			continue
		}
		dstOff := rOff - off
		dstSize := lrr.pointerSize
		srcOff := uint64(0)
		if rOff < off {
			dstOff = 0
			srcOff = off - rOff // always < frw.pointerSize
			dstSize -= srcOff
		}
		if rOff+dstSize > max {
			dstSize -= rOff + dstSize - max
		}

		// cehck that the read content is the expected ones ( Rebase.Raw() )
		lrr.writePointer(buf, r.Raw())
		if bytes.Compare(buf[srcOff:srcOff+dstSize], p[dstOff:dstOff+dstSize]) != 0 {
			// this shall be a warning, we lack a logging system
			return fmt.Errorf("underlying read value at %x is %x, expected %x", rOff, p[dstOff:dstOff+dstSize], buf[srcOff:srcOff+dstSize])
		}
		lrr.writePointer(buf, r.Resolve(lrr.baseAddr))
		copy(p[dstOff:dstOff+dstSize], buf[srcOff:])
	}

	return nil
}
