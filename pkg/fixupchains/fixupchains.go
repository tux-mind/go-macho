package fixupchains

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/blacktop/go-macho/types"
)

// NewChainedFixups creates a new DyldChainedFixups instance
func NewChainedFixups(lcdat *bytes.Reader, sr *types.MachoReader, bo binary.ByteOrder) *DyldChainedFixups {
	return &DyldChainedFixups{
		r:  lcdat,
		sr: *sr,
		bo: bo,
	}
}

// Parse parses a LC_DYLD_CHAINED_FIXUPS load command
func (dcf *DyldChainedFixups) Parse() (*DyldChainedFixups, error) {

	if dcf.Starts == nil {
		if err := dcf.ParseStarts(); err != nil {
			return nil, err
		}
	}

	// Parse Imports
	dcf.parseImports()

	for segIdx, start := range dcf.Starts {

		if start.PageStarts == nil {
			continue
		}

		for pageIndex := uint16(0); pageIndex < start.DyldChainedStartsInSegment.PageCount; pageIndex++ {
			offsetInPage := start.PageStarts[pageIndex]

			if offsetInPage == DYLD_CHAINED_PTR_START_NONE {
				continue
			}

			if offsetInPage&DYLD_CHAINED_PTR_START_MULTI != 0 {
				// 32-bit chains which may need multiple starts per page
				overflowIndex := offsetInPage & ^DYLD_CHAINED_PTR_START_MULTI
				chainEnd := false
				for !chainEnd {
					chainEnd = (start.PageStarts[overflowIndex]&DYLD_CHAINED_PTR_START_LAST != 0)
					offsetInPage = (start.PageStarts[overflowIndex] & ^DYLD_CHAINED_PTR_START_LAST)
					if err := dcf.walkDcFixupChain(segIdx, pageIndex, offsetInPage); err != nil {
						return nil, err
					}
					overflowIndex++
				}

			} else {
				// one chain per page
				if err := dcf.walkDcFixupChain(segIdx, pageIndex, offsetInPage); err != nil {
					return nil, err
				}
			}
		}
	}

	return dcf, nil
}

// ParseStarts parses the DyldChainedStartsInSegment(s)
func (dcf *DyldChainedFixups) ParseStarts() error {

	if err := binary.Read(dcf.r, dcf.bo, &dcf.DyldChainedFixupsHeader); err != nil {
		return err
	}

	dcf.r.Seek(int64(dcf.DyldChainedFixupsHeader.StartsOffset), io.SeekStart)

	var segCount uint32
	if err := binary.Read(dcf.r, dcf.bo, &segCount); err != nil {
		return err
	}

	dcf.Starts = make([]DyldChainedStarts, segCount)
	segInfoOffsets := make([]uint32, segCount)
	if err := binary.Read(dcf.r, dcf.bo, &segInfoOffsets); err != nil {
		return err
	}

	for segIdx, segInfoOffset := range segInfoOffsets {
		if segInfoOffset == 0 {
			continue
		}

		dcf.r.Seek(int64(dcf.DyldChainedFixupsHeader.StartsOffset+segInfoOffset), io.SeekStart)
		if err := binary.Read(dcf.r, dcf.bo, &dcf.Starts[segIdx].DyldChainedStartsInSegment); err != nil {
			return err
		}

		dcf.Starts[segIdx].PageStarts = make([]DCPtrStart, dcf.Starts[segIdx].DyldChainedStartsInSegment.PageCount)
		if err := binary.Read(dcf.r, dcf.bo, &dcf.Starts[segIdx].PageStarts); err != nil {
			return err
		}
	}

	return nil
}

func (dcf *DyldChainedFixups) GetImportForPointer(pointer uint64) (*DcfImport, error) {
	// this is a best effort approach,
	// ideally we should know the address at which the pointer came from
	// to localise the dcf page responsable for it
	// ref: https://github.com/PureDarwin/dyld/blob/56cd028e61d0d487e5b604eb6bd2c5d577b24241/src/ImageLoaderMachOCompressed.cpp#L1037
	// ref: https://github.com/PureDarwin/dyld/blob/56cd028e61d0d487e5b604eb6bd2c5d577b24241/dyld3/MachOLoaded.cpp#L1047
	var ordinal uint64

	for _, s := range dcf.Starts {
		if s.PageCount == 0 {
			continue
		}
		switch s.PointerFormat {
		case DYLD_CHAINED_PTR_32:
			ptr := uint32(pointer)
			if !Generic32IsBind(ptr) {
				continue
			}
			bind := DyldChainedPtr32Bind{Pointer: ptr}
			ordinal = bind.Ordinal()
		case DYLD_CHAINED_PTR_64:
			if !Generic64IsBind(pointer) {
				continue
			}
			bind := DyldChainedPtr64Bind{Pointer: pointer}
			ordinal = bind.Ordinal()
		case DYLD_CHAINED_PTR_ARM64E:
			if !DcpArm64eIsBind(pointer) {
				continue
			}
			if DcpArm64eIsAuth(pointer) {
				authBind := DyldChainedPtrArm64eAuthBind{Pointer: pointer}
				ordinal = authBind.Ordinal()
			} else {
				bind := DyldChainedPtrArm64eBind{Pointer: pointer}
				ordinal = bind.Ordinal()
			}
		default:
			continue
		}
		if ordinal < uint64(len(dcf.Imports)) {
			return &dcf.Imports[ordinal], nil
		}
	}
	return nil, fmt.Errorf("not a bind pointer")
}

func (dcf *DyldChainedFixups) RebasePointer(preferredLoadAddress uint64, pointer uint64) uint64 {
	// see notes of GetImportsForPointer
	// ref: https://github.com/PureDarwin/dyld/blob/56cd028e61d0d487e5b604eb6bd2c5d577b24241/dyld3/MachOLoaded.cpp#L1014
	for _, s := range dcf.Starts {
		if s.PageCount == 0 {
			continue
		}
		switch s.PointerFormat {
		case DYLD_CHAINED_PTR_32:
			ptr := uint32(pointer)
			if !Generic32IsBind(ptr) {
				rebase := DyldChainedPtr32Rebase{Pointer: ptr}
				return rebase.Target()
			}
		case DYLD_CHAINED_PTR_64:
			if !Generic64IsBind(pointer) {
				rebase := DyldChainedPtr64Rebase{Pointer: pointer}
				return uint64(rebase.UnpackedTarget())
			}
		case DYLD_CHAINED_PTR_ARM64E:
			if !DcpArm64eIsBind(pointer) {
				if DcpArm64eIsAuth(pointer) {
					authRebase := DyldChainedPtrArm64eAuthRebase{Pointer: pointer}
					return authRebase.Target() + preferredLoadAddress
				} else {
					rebase := DyldChainedPtrArm64eRebase{Pointer: pointer}
					return rebase.UnpackTarget()
				}
			}
		}
	}
	return pointer
}

func (dcf *DyldChainedFixups) walkDcFixupChain(segIdx int, pageIndex uint16, offsetInPage DCPtrStart) error {

	var dcPtr uint32
	var dcPtr64 uint64
	var next uint64

	chainEnd := false
	segOffset := dcf.Starts[segIdx].DyldChainedStartsInSegment.SegmentOffset
	pageContentStart := segOffset + uint64(pageIndex)*uint64(dcf.Starts[segIdx].DyldChainedStartsInSegment.PageSize)

	for !chainEnd {
		fixupLocation := pageContentStart + uint64(offsetInPage) + next
		dcf.sr.Seek(int64(fixupLocation), io.SeekStart)

		pointerFormat := dcf.Starts[segIdx].DyldChainedStartsInSegment.PointerFormat

		switch pointerFormat {
		case DYLD_CHAINED_PTR_32:
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr); err != nil {
				return err
			}
			if Generic32IsBind(dcPtr) {
				bind := DyldChainedPtr32Bind{Pointer: dcPtr, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			} else {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr32Rebase{
					Pointer: dcPtr,
					Fixup:   fixupLocation,
				})
			}
			if Generic32Next(dcPtr) == 0 {
				chainEnd = true
			}
			next += Generic32Next(dcPtr) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_32_CACHE:
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr); err != nil {
				return err
			}
			dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr32CacheRebase{
				Pointer: dcPtr,
				Fixup:   fixupLocation,
			})
			if Generic32Next(dcPtr) == 0 {
				chainEnd = true
			}
			next += Generic32Next(dcPtr) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_32_FIRMWARE:
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr); err != nil {
				return err
			}
			dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr32FirmwareRebase{
				Pointer: dcPtr,
				Fixup:   fixupLocation,
			})
			if Generic32Next(dcPtr) == 0 {
				chainEnd = true
			}
			next += Generic32Next(dcPtr) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_64: // target is vmaddr
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			if Generic64IsBind(dcPtr64) {
				bind := DyldChainedPtr64Bind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			} else {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr64Rebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			}
			if Generic64Next(dcPtr64) == 0 {
				chainEnd = true
			}
			next += Generic64Next(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_64_OFFSET: // target is vm offset
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr64RebaseOffset{
				Pointer: dcPtr64,
				Fixup:   fixupLocation,
			})
			if Generic64Next(dcPtr64) == 0 {
				chainEnd = true
			}
			next += Generic64Next(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_64_KERNEL_CACHE:
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr64KernelCacheRebase{
				Pointer: dcPtr64,
				Fixup:   fixupLocation,
			})
			if Generic64Next(dcPtr64) == 0 {
				chainEnd = true
			}
			next += Generic64Next(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_X86_64_KERNEL_CACHE: // stride 1, x86_64 kernel caches
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtr64KernelCacheRebase{
				Pointer: dcPtr64,
				Fixup:   fixupLocation,
			})
			if Generic64Next(dcPtr64) == 0 {
				chainEnd = true
			}
			next += Generic64Next(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_ARM64E_KERNEL: // stride 4, unauth target is vm offset
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			if !DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eRebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else if DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				bind := DyldChainedPtrArm64eBind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			} else if !DcpArm64eIsBind(dcPtr64) && DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eAuthRebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else {
				bind := DyldChainedPtrArm64eAuthBind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			}
			if DcpArm64eNext(dcPtr64) == 0 {
				chainEnd = true
			}
			next += DcpArm64eNext(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_ARM64E_FIRMWARE: // stride 4, unauth target is vmaddr
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			if !DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eRebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else if DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				bind := DyldChainedPtrArm64eBind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			} else if !DcpArm64eIsBind(dcPtr64) && DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eAuthRebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else {
				bind := DyldChainedPtrArm64eAuthBind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			}
			if DcpArm64eNext(dcPtr64) == 0 {
				chainEnd = true
			}
			next += DcpArm64eNext(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_ARM64E: // stride 8, unauth target is vmaddr
			fallthrough
		case DYLD_CHAINED_PTR_ARM64E_USERLAND: // stride 8, unauth target is vm offset
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			if !DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eRebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else if DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				bind := DyldChainedPtrArm64eBind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			} else if !DcpArm64eIsBind(dcPtr64) && DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eAuthRebase{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else {
				bind := DyldChainedPtrArm64eAuthBind{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			}
			if DcpArm64eNext(dcPtr64) == 0 {
				chainEnd = true
			}
			next += DcpArm64eNext(dcPtr64) * stride(pointerFormat)
		case DYLD_CHAINED_PTR_ARM64E_USERLAND24: // stride 8, unauth target is vm offset, 24-bit bind
			if err := binary.Read(dcf.sr, dcf.bo, &dcPtr64); err != nil {
				return err
			}
			if !DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eRebase24{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else if DcpArm64eIsBind(dcPtr64) && DcpArm64eIsAuth(dcPtr64) {
				bind := DyldChainedPtrArm64eAuthBind24{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			} else if !DcpArm64eIsBind(dcPtr64) && DcpArm64eIsAuth(dcPtr64) {
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, DyldChainedPtrArm64eAuthRebase24{
					Pointer: dcPtr64,
					Fixup:   fixupLocation,
				})
			} else if DcpArm64eIsBind(dcPtr64) && !DcpArm64eIsAuth(dcPtr64) {
				bind := DyldChainedPtrArm64eBind24{Pointer: dcPtr64, Fixup: fixupLocation}
				bind.Import = dcf.Imports[bind.Ordinal()].Name
				dcf.Starts[segIdx].Fixups = append(dcf.Starts[segIdx].Fixups, bind)
			}
			if DcpArm64eNext(dcPtr64) == 0 {
				chainEnd = true
			}
			next += DcpArm64eNext(dcPtr64) * stride(pointerFormat)
		default:
			return fmt.Errorf("unknown pointer format %#04X", dcf.Starts[segIdx].DyldChainedStartsInSegment.PointerFormat)
		}
	}

	return nil
}

func (dcf *DyldChainedFixups) parseImports() error {

	var imports []Import

	dcf.r.Seek(int64(dcf.ImportsOffset), io.SeekStart)

	switch dcf.DyldChainedFixupsHeader.ImportsFormat {
	case DC_IMPORT:
		ii := make([]DyldChainedImport, dcf.ImportsCount)
		if err := binary.Read(dcf.r, dcf.bo, &ii); err != nil {
			return err
		}
		for _, i := range ii {
			imports = append(imports, i)
		}
	case DC_IMPORT_ADDEND:
		ii := make([]DyldChainedImportAddend, dcf.ImportsCount)
		if err := binary.Read(dcf.r, dcf.bo, &ii); err != nil {
			return err
		}
		for _, i := range ii {
			imports = append(imports, i)
		}
	case DC_IMPORT_ADDEND64:
		ii := make([]DyldChainedImportAddend64, dcf.ImportsCount)
		if err := binary.Read(dcf.r, dcf.bo, &ii); err != nil {
			return err
		}
		for _, i := range ii {
			imports = append(imports, i)
		}
	}

	symbolsPool := io.NewSectionReader(dcf.r, int64(dcf.SymbolsOffset), dcf.r.Size()-int64(dcf.SymbolsOffset))
	for _, i := range imports {
		symbolsPool.Seek(int64(i.NameOffset()), io.SeekStart)
		s, err := bufio.NewReader(symbolsPool).ReadString('\x00')
		if err != nil {
			return fmt.Errorf("failed to read string at: %d: %v", uint64(dcf.SymbolsOffset)+i.NameOffset(), err)
		}
		dcf.Imports = append(dcf.Imports, DcfImport{
			Name:   strings.Trim(s, "\x00"),
			Import: i,
		})
	}

	return nil
}
