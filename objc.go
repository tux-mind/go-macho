package macho

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/blacktop/go-macho/types"
	"github.com/blacktop/go-macho/types/objc"
)

var ErrObjcSectionNotFound = errors.New("missing required ObjC section")

// TODO refactor into a pkg

// HasObjC returns true if MachO contains a __objc_imageinfo section
func (f *File) HasObjC() bool {
	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_imageinfo"); sec != nil {
				return true
			}
		}
	}
	if f.CPU == types.CPU386 {
		if sec := f.Section("__OBJC", "__image_info"); sec != nil {
			return true
		}
	}
	return false
}

// HasPlusLoadMethod returns true if MachO contains a __objc_nlclslist or __objc_nlcatlist section
func (f *File) HasPlusLoadMethod() bool {
	// TODO add the old way of detecting from dyld3/MachOAnalyzer.cpp
	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_nlclslist"); sec != nil {
				return true
			}
			if sec := f.Section(s.Name, "__objc_nlcatlist"); sec != nil {
				return true
			}
		}
	}
	return false
}

// HasObjCMessageReferences returns true if MachO contains a __objc_msgrefs section
func (f *File) HasObjCMessageReferences() bool {
	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			for j := uint32(0); j < s.Nsect; j++ {
				c := f.FileTOC.Sections[j+s.Firstsect]
				if strings.EqualFold("__objc_msgrefs", c.Name) {
					return true
				}
			}
		}
	}
	return false
}

// GetObjCToc returns a table of contents of the ObjC objects in the MachO
func (f *File) GetObjCToc() objc.Toc {
	var oInfo objc.Toc
	for _, sec := range f.FileTOC.Sections {
		if strings.HasPrefix(sec.SectionHeader.Seg, "__DATA") {
			switch sec.Name {
			case "__objc_classlist":
				oInfo.ClassList = sec.Size / f.pointerSize()
			case "__objc_nlclslist":
				oInfo.NonLazyClassList = sec.Size / f.pointerSize()
			case "__objc_catlist":
				oInfo.CatList = sec.Size / f.pointerSize()
			case "__objc_nlcatlist":
				oInfo.NonLazyCatList = sec.Size / f.pointerSize()
			case "__objc_protolist":
				oInfo.ProtoList = sec.Size / f.pointerSize()
			case "__objc_classrefs":
				oInfo.ClassRefs = sec.Size / f.pointerSize()
			case "__objc_superrefs":
				oInfo.SuperRefs = sec.Size / f.pointerSize()
			case "__objc_selrefs":
				oInfo.SelRefs = sec.Size / f.pointerSize()
			}
		} else if (f.CPU == types.CPU386) && strings.EqualFold(sec.Name, "__OBJC") {
			if strings.EqualFold(sec.Name, "__message_refs") {
				oInfo.SelRefs += sec.SectionHeader.Size / 4
			} else if strings.EqualFold(sec.Name, "__class") {
				oInfo.ClassList += sec.SectionHeader.Size / 48
			} else if strings.EqualFold(sec.Name, "__protocol") {
				oInfo.ProtoList += sec.SectionHeader.Size / 20
			}
		}
	}
	return oInfo
}

// GetObjCImageInfo returns the parsed __objc_imageinfo data
func (f *File) GetObjCImageInfo() (*objc.ImageInfo, error) {
	var imgInfo objc.ImageInfo
	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_imageinfo"); sec != nil {
				off, err := f.vma.GetOffset(f.vma.Convert(sec.Addr))
				if err != nil {
					return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
				}
				f.rr.Seek(int64(off), io.SeekStart)

				dat := make([]byte, sec.Size)
				if err := binary.Read(f.rr, f.ByteOrder, dat); err != nil {
					return nil, fmt.Errorf("failed to read %s.%s data: %v", sec.Seg, sec.Name, err)
				}

				if err := binary.Read(bytes.NewReader(dat), f.ByteOrder, &imgInfo); err != nil {
					return nil, fmt.Errorf("failed to read %T: %v", imgInfo, err)
				}

				return &imgInfo, nil
			}
		}
	}
	return nil, fmt.Errorf("macho does not contain __objc_imageinfo section: %w", ErrObjcSectionNotFound)
}

// GetObjCClassInfo returns the ClassRO64 (class_ro_t) for a given virtual memory address
func (f *File) GetObjCClassInfo(vmaddr uint64) (*objc.ClassRO64, error) {
	var classData objc.ClassRO64

	off, err := f.vma.GetOffset(vmaddr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	if err := binaryReadStruct[classRO](f, &classData); err != nil {
		return nil, fmt.Errorf("failed to read %T: %v", classData, err)
	}

	// slide pointers
	classData.IvarLayoutVMAddr = f.vma.Convert(classData.IvarLayoutVMAddr)
	classData.NameVMAddr = f.vma.Convert(classData.NameVMAddr)
	classData.BaseMethodsVMAddr = f.vma.Convert(classData.BaseMethodsVMAddr)
	classData.BaseProtocolsVMAddr = f.vma.Convert(classData.BaseProtocolsVMAddr)
	classData.IvarsVMAddr = f.vma.Convert(classData.IvarsVMAddr)
	classData.WeakIvarLayoutVMAddr = f.vma.Convert(classData.WeakIvarLayoutVMAddr)
	classData.BasePropertiesVMAddr = f.vma.Convert(classData.BasePropertiesVMAddr)

	return &classData, nil
}

// GetObjCClassNames returns a map of class names to their section data virtual memory address
func (f *File) GetObjCClassNames() (map[string]uint64, error) {
	class2vmaddr := make(map[string]uint64)

	if sec := f.Section("__TEXT", "__objc_classname"); sec != nil { // Names for locally implemented classes
		off, err := f.vma.GetOffset(f.vma.Convert(sec.Addr))
		if err != nil {
			return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
		}
		f.rr.Seek(int64(off), io.SeekStart)

		dat := make([]byte, sec.Size)
		if err := binary.Read(f.rr, f.ByteOrder, dat); err != nil {
			return nil, fmt.Errorf("failed to read %s.%s data: %v", sec.Seg, sec.Name, err)
		}

		r := bytes.NewBuffer(dat)

		for {
			s, err := r.ReadString('\x00')
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read from class name string pool: %v", err)
			}
			class2vmaddr[strings.Trim(s, "\x00")] = sec.Addr + (sec.Size - uint64(r.Len()+len(s)))
		}
	}

	return class2vmaddr, nil
}

// GetObjCMethodNames returns a map of method names to their section data virtual memory address
func (f *File) GetObjCMethodNames() (map[string]uint64, error) {
	meth2vmaddr := make(map[string]uint64)

	if sec := f.Section("__TEXT", "__objc_methname"); sec != nil { // Method names for locally implemented methods
		off, err := f.vma.GetOffset(f.vma.Convert(sec.Addr))
		if err != nil {
			return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
		}
		f.rr.Seek(int64(off), io.SeekStart)

		dat := make([]byte, sec.Size)
		if err := binary.Read(f.rr, f.ByteOrder, dat); err != nil {
			return nil, fmt.Errorf("failed to read %s.%s data: %v", sec.Seg, sec.Name, err)
		}

		r := bytes.NewBuffer(dat)

		for {
			s, err := r.ReadString('\x00')
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to read from method name string pool: %v", err)
			}
			meth2vmaddr[strings.Trim(s, "\x00")] = sec.Addr + (sec.Size - uint64(r.Len()+len(s)))
		}
	}

	return meth2vmaddr, nil
}

// read a pointer from the specified offset
func (f *File) ReadPointer(offset uint64) (ptr uint64, err error) {
	ptrSize := f.pointerSize()
	var buf = make([]byte, ptrSize)
	if _, err = f.rr.ReadAt(buf, int64(offset)); err == nil {
		if ptrSize == 4 {
			ptr = uint64(f.ByteOrder.Uint32(buf))
		} else {
			ptr = f.ByteOrder.Uint64(buf)
		}
	}
	return
}

// read a list of pointers from a section
func (f *File) readPointersFromSection(sec *Section) (ptrs []uint64, err error) {
	off, err := f.vma.GetOffset(f.vma.Convert(sec.Addr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	return readPointers(f, sec.Size/f.pointerSize())
}

// GetObjCClasses returns an array of Objective-C classes
func (f *File) GetObjCClasses() ([]*objc.Class, error) {
	var classes []*objc.Class

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_classlist"); sec != nil { // An array of pointers to ObjC classes
				ptrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s pointers: %v", sec.Name, err)
				}

				for _, ptr := range ptrs {
					ptr = f.vma.Convert(ptr)
					if c, ok := f.objc[ptr]; ok {
						classes = append(classes, c)
					} else {
						class, err := f.GetObjCClass(ptr)
						if err != nil {
							return nil, fmt.Errorf("failed to read objc_class_t at vmaddr %#x: %v", ptr, err)
						}
						classes = append(classes, class)
						f.objc[ptr] = class
					}
				}
			}
		}
	}

	return classes, nil
}

// GetObjCNonLazyClasses returns an array of Objective-C classes that implement +load
func (f *File) GetObjCNonLazyClasses() ([]*objc.Class, error) {
	var classes []*objc.Class

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_nlclslist"); sec != nil { // An array of pointers to classes who implement +load
				ptrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s pointers: %v", sec.Name, err)
				}

				for _, ptr := range ptrs {
					ptr = f.vma.Convert(ptr)
					if c, ok := f.objc[ptr]; ok {
						classes = append(classes, c)
					} else {
						class, err := f.GetObjCClass(ptr)
						if err != nil {
							return nil, fmt.Errorf("failed to read objc_class_t at vmaddr %#x: %v", ptr, err)
						}
						classes = append(classes, class)
						f.objc[ptr] = class
					}
				}
			}
		}
	}

	return classes, nil
}

// GetObjCClass parses an Objective-C class at a given virtual memory address
func (f *File) GetObjCClass(vmaddr uint64) (*objc.Class, error) {
	var classPtr objc.SwiftClassMetadata64

	if c, ok := f.objc[vmaddr]; ok {
		return c, nil
	}

	off, err := f.vma.GetOffset(vmaddr)
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	if err := binaryReadStruct[swiftClassMetadata](f, &classPtr); err != nil {
		return nil, fmt.Errorf("failed to read %T: %v", classPtr, err)
	}

	classPtr.IsaVMAddr = f.vma.Convert(classPtr.IsaVMAddr)
	classPtr.SuperclassVMAddr = f.vma.Convert(classPtr.SuperclassVMAddr)
	classPtr.MethodCacheBuckets = f.vma.Convert(classPtr.MethodCacheBuckets)
	classPtr.MethodCacheProperties = f.vma.Convert(classPtr.MethodCacheProperties)
	classPtr.DataVMAddrAndFastFlags = f.vma.Convert(classPtr.DataVMAddrAndFastFlags)

	classDataVMAddr := classPtr.DataVMAddrAndFastFlags & objc.FAST_DATA_MASK64
	if !f.is64bit() {
		classDataVMAddr = classPtr.DataVMAddrAndFastFlags & objc.FAST_DATA_MASK
	}

	info, err := f.GetObjCClassInfo(classDataVMAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get class info at vmaddr: %#x; %v", classDataVMAddr, err)
	}

	name, err := f.GetCString(info.NameVMAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to read cstring: %v", err)
	}

	var methods []objc.Method
	if info.BaseMethodsVMAddr > 0 {
		methods, err = f.GetObjCMethods(info.BaseMethodsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to get methods at vmaddr: %#x; %v", info.BaseMethodsVMAddr, err)
		}
	}

	var prots []objc.Protocol
	if info.BaseProtocolsVMAddr > 0 {
		prots, err = f.parseObjcProtocolList(info.BaseProtocolsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read protocols vmaddr: %#x; %v", info.BaseProtocolsVMAddr, err)
		}
	}

	var ivars []objc.Ivar
	if info.IvarsVMAddr > 0 {
		ivars, err = f.GetObjCIvars(info.IvarsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to get ivars at vmaddr: %#x; %v", info.IvarsVMAddr, err)
		}
	}

	var props []objc.Property
	if info.BasePropertiesVMAddr > 0 {
		props, err = f.GetObjCProperties(info.BasePropertiesVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to get props at vmaddr: %#x; %v", info.BasePropertiesVMAddr, err)
		}
	}

	superClass := &objc.Class{}
	if classPtr.SuperclassVMAddr > 0 {
		if info.Flags.IsRoot() {
			superClass = &objc.Class{Name: "<ROOT>"}
		} else if info.Flags.IsMeta() {
			superClass = &objc.Class{Name: "<META>"}
			// } else if info.Flags > 0 {
		} else {
			if c, ok := f.objc[classPtr.SuperclassVMAddr]; ok {
				superClass = c
			} else {
				superClass, err = f.GetObjCClass(classPtr.SuperclassVMAddr)
				if err != nil {
					if f.HasFixups() {
						bindName, err := f.GetBindName(classPtr.SuperclassVMAddr)
						if err == nil {
							superClass = &objc.Class{Name: strings.TrimPrefix(bindName, "_OBJC_CLASS_$_")}
						} else {
							return nil, fmt.Errorf("failed to read super class objc_class_t at vmaddr: %#x; %v", vmaddr, err)
						}
					} else {
						superClass = &objc.Class{}
					}
				}
				f.objc[classPtr.SuperclassVMAddr] = superClass
			}
		}
	}

	isaClass := &objc.Class{}
	var cMethods []objc.Method
	if classPtr.IsaVMAddr > 0 {
		if !info.Flags.IsMeta() {
			if c, ok := f.objc[classPtr.IsaVMAddr]; ok {
				isaClass = c
			} else {
				isaClass, err = f.GetObjCClass(classPtr.IsaVMAddr)
				if err != nil {
					if f.HasFixups() {
						bindName, err := f.GetBindName(classPtr.IsaVMAddr)
						if err == nil {
							isaClass = &objc.Class{Name: strings.TrimPrefix(bindName, "_OBJC_CLASS_$_")}
						} else {
							return nil, fmt.Errorf("failed to read super class objc_class_t at vmaddr: %#x; %v", vmaddr, err)
						}
					} else {
						isaClass = &objc.Class{}
					}
				} else {
					if isaClass.ReadOnlyData.Flags.IsMeta() {
						cMethods = isaClass.InstanceMethods
					}
				}
				f.objc[classPtr.IsaVMAddr] = isaClass
			}
		}
	}

	return &objc.Class{
		Name:                  name,
		SuperClass:            superClass.Name,
		Isa:                   isaClass.Name,
		InstanceMethods:       methods,
		ClassMethods:          cMethods,
		Ivars:                 ivars,
		Props:                 props,
		Prots:                 prots,
		ClassPtr:              vmaddr,
		IsaVMAddr:             classPtr.IsaVMAddr,
		SuperclassVMAddr:      classPtr.SuperclassVMAddr,
		MethodCacheBuckets:    classPtr.MethodCacheBuckets,
		MethodCacheProperties: classPtr.MethodCacheProperties,
		DataVMAddr:            classDataVMAddr,
		IsSwiftLegacy:         (classPtr.DataVMAddrAndFastFlags&objc.FAST_IS_SWIFT_LEGACY == 1),
		IsSwiftStable:         (classPtr.DataVMAddrAndFastFlags&objc.FAST_IS_SWIFT_STABLE == 1),
		ReadOnlyData:          *info,
	}, nil
}

// GetObjCCategories returns an array of Objective-C categories
func (f *File) GetObjCCategories() ([]objc.Category, error) {
	var categoryPtr objc.CategoryT
	var categories []objc.Category

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_catlist"); sec != nil { // List of ObjC categories
				ptrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s.%s pointers: %v", sec.Seg, sec.Name, err)
				}

				for _, ptr := range ptrs {
					ptr = f.vma.Convert(ptr)

					off, err := f.vma.GetOffset(ptr)
					if err != nil {
						return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
					}
					f.rr.Seek(int64(off), io.SeekStart)

					if err := binaryReadStruct[category32T](f, &categoryPtr); err != nil {
						return nil, fmt.Errorf("failed to read %T: %v", categoryPtr, err)
					}

					category := objc.Category{VMAddr: ptr}

					categoryPtr.NameVMAddr = f.vma.Convert(categoryPtr.NameVMAddr)
					category.Name, err = f.GetCString(categoryPtr.NameVMAddr)
					if err != nil {
						return nil, fmt.Errorf("failed to read cstring: %v", err)
					}
					if categoryPtr.ClsVMAddr > 0 {
						categoryPtr.ClsVMAddr = f.vma.Convert(categoryPtr.ClsVMAddr)
						if c, ok := f.objc[categoryPtr.ClsVMAddr]; ok {
							category.Class = c
						} else {
							category.Class, err = f.GetObjCClass(categoryPtr.ClsVMAddr)
							if err != nil {
								if f.HasFixups() {
									bindName, err := f.GetBindName(categoryPtr.ClsVMAddr)
									if err == nil {
										category.Class = &objc.Class{Name: strings.TrimPrefix(bindName, "_OBJC_CLASS_$_")}
									} else {
										return nil, fmt.Errorf("failed to read super class objc_class_t at vmaddr: %#x; %v", categoryPtr.ClsVMAddr, err)
									}
								} else {
									category.Class = &objc.Class{}
								}
							}
							f.objc[categoryPtr.ClsVMAddr] = category.Class
						}
					}
					if categoryPtr.InstanceMethodsVMAddr > 0 {
						categoryPtr.InstanceMethodsVMAddr = f.vma.Convert(categoryPtr.InstanceMethodsVMAddr)
						category.InstanceMethods, err = f.GetObjCMethods(categoryPtr.InstanceMethodsVMAddr)
						if err != nil {
							return nil, fmt.Errorf("failed to get instance methods at vmaddr: %#x; %v", categoryPtr.InstanceMethodsVMAddr, err)
						}
					}
					if categoryPtr.ClassMethodsVMAddr > 0 {
						categoryPtr.ClassMethodsVMAddr = f.vma.Convert(categoryPtr.ClassMethodsVMAddr)
						category.ClassMethods, err = f.GetObjCMethods(categoryPtr.ClassMethodsVMAddr)
						if err != nil {
							return nil, fmt.Errorf("failed to get class methods at vmaddr: %#x; %v", categoryPtr.ClassMethodsVMAddr, err)
						}
					}
					if categoryPtr.ProtocolsVMAddr > 0 {
						categoryPtr.ProtocolsVMAddr = f.vma.Convert(categoryPtr.ProtocolsVMAddr)
						// category.Protocol, err = f.getObjcProtocol(categoryPtr.ProtocolsVMAddr)
						// if err != nil {
						// 	return nil, fmt.Errorf("failed to get protocols at vmaddr: %#x; %v", categoryPtr.ClassMethodsVMAddr, err)
						// }
					}
					if categoryPtr.InstancePropertiesVMAddr > 0 {
						categoryPtr.InstancePropertiesVMAddr = f.vma.Convert(categoryPtr.InstancePropertiesVMAddr)
						category.Properties, err = f.GetObjCProperties(categoryPtr.InstancePropertiesVMAddr)
						if err != nil {
							return nil, fmt.Errorf("failed to get class methods at vmaddr: %#x; %v", categoryPtr.ClassMethodsVMAddr, err)
						}
					}

					category.CategoryT = categoryPtr
					categories = append(categories, category)
				}
			}
		}
	}

	return categories, nil
}

// GetCFStrings returns the Objective-C CFStrings
func (f *File) GetCFStrings() ([]objc.CFString, error) {

	var cfstrings []objc.CFString

	for _, s := range f.Segments() {
		if sec := f.Section(s.Name, "__cfstring"); sec != nil {
			cfStrTypes, err := readStructsFromSection[cfstring32T, objc.CFString64T](f, sec)
			if err != nil {
				return nil, fmt.Errorf("failed to read %T structs: %v", cfStrTypes, err)
			} else if len(cfStrTypes) == 0 {
				continue
			}

			structSize := sec.Size / uint64(len(cfStrTypes))

			for idx, cfstr := range cfStrTypes {
				var cfstring objc.CFString

				cfstr.IsaVMAddr = f.vma.Convert(cfstr.IsaVMAddr)
				cfstr.Data = f.vma.Convert(cfstr.Data)
				cfstring.CFString64T = &cfstr
				if cfstr.Data == 0 {
					return nil, fmt.Errorf("unhandled cstring parse case where data is 0") // TODO: finish this
					// uint64_t n_value;
					// const char *symbol_name = get_symbol_64(offset + offsetof(struct cfstring64_t, characters), S, info, n_value);
					// if (symbol_name == nullptr)
					//   return nullptr;
					// cfs_characters = n_value;
				}
				cfstring.Name, err = f.GetCString(cfstr.Data)
				if err != nil {
					return nil, fmt.Errorf("failed to read cstring: %v", err)
				}
				if c, ok := f.objc[cfstr.IsaVMAddr]; ok {
					cfstring.Class = c
				}
				cfstring.Address = sec.Addr + uint64(uint64(idx)*structSize)
				if err != nil {
					return nil, fmt.Errorf("failed to calulate cfstring vmaddr: %v", err)
				}
				cfstrings = append(cfstrings, cfstring)
			}
		}
	}

	return cfstrings, nil
}

func (f *File) parseObjcProtocolList(vmaddr uint64) ([]objc.Protocol, error) {
	var protocols []objc.Protocol

	off, err := f.vma.GetOffset(f.vma.Convert(vmaddr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	var protList objc.ProtocolList

	if protList.Count, err = readPointer(f); err != nil {
		return nil, fmt.Errorf("failed to read protocol_list_t count: %v", err)
	}
	if protList.Protocols, err = readPointers(f, protList.Count); err != nil {
		return nil, fmt.Errorf("failed to read protocol_list_t prots: %v", err)
	}

	protocols = make([]objc.Protocol, protList.Count)

	for i, protPtr := range protList.Protocols {
		prot, err := f.getObjcProtocol(f.vma.Convert(protPtr))
		if err != nil {
			return nil, err
		}
		protocols[i] = *prot
	}

	return protocols, nil
}

func (f *File) getObjcProtocol(vmaddr uint64) (proto *objc.Protocol, err error) {
	var protoPtr objc.ProtocolT

	off, err := f.vma.GetOffset(f.vma.Convert(vmaddr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	if err := binaryReadStruct[protocol32T](f, &protoPtr); err != nil {
		return nil, fmt.Errorf("failed to read protocol_t: %v", err)
	}

	proto = &objc.Protocol{Ptr: vmaddr}

	if protoPtr.NameVMAddr > 0 {
		protoPtr.NameVMAddr = f.vma.Convert(protoPtr.NameVMAddr)
		proto.Name, err = f.GetCString(protoPtr.NameVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read cstring: %v", err)
		}
	}
	if protoPtr.IsaVMAddr > 0 {
		protoPtr.IsaVMAddr = f.vma.Convert(protoPtr.IsaVMAddr)
		if c, ok := f.objc[protoPtr.IsaVMAddr]; ok {
			proto.Isa = c
		} else {
			// FIXME: causes infinite loop
			// proto.Isa, err = f.GetObjCClass(protoPtr.IsaVMAddr)
			// if err != nil {
			// 	return nil, fmt.Errorf("failed to get class at vmaddr: %#x; %v", protoPtr.IsaVMAddr, err)
			// }
			// f.objc[protoPtr.IsaVMAddr] = proto.Isa
		}
	}
	if protoPtr.ProtocolsVMAddr > 0 {
		protoPtr.ProtocolsVMAddr = f.vma.Convert(protoPtr.ProtocolsVMAddr)
		proto.Prots, err = f.parseObjcProtocolList(protoPtr.ProtocolsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read protocols vmaddr: %v", err)
		}
	}
	if protoPtr.InstanceMethodsVMAddr > 0 {
		protoPtr.InstanceMethodsVMAddr = f.vma.Convert(protoPtr.InstanceMethodsVMAddr)
		proto.InstanceMethods, err = f.GetObjCMethods(protoPtr.InstanceMethodsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read instance method vmaddr: %v", err)
		}
	}
	if protoPtr.OptionalInstanceMethodsVMAddr > 0 {
		protoPtr.OptionalInstanceMethodsVMAddr = f.vma.Convert(protoPtr.OptionalInstanceMethodsVMAddr)
		proto.OptionalInstanceMethods, err = f.GetObjCMethods(protoPtr.OptionalInstanceMethodsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read optional instance method vmaddr: %v", err)
		}
	}
	if protoPtr.ClassMethodsVMAddr > 0 {
		protoPtr.ClassMethodsVMAddr = f.vma.Convert(protoPtr.ClassMethodsVMAddr)
		proto.ClassMethods, err = f.GetObjCMethods(protoPtr.ClassMethodsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read class method vmaddr: %v", err)
		}
	}
	if protoPtr.OptionalClassMethodsVMAddr > 0 {
		protoPtr.OptionalClassMethodsVMAddr = f.vma.Convert(protoPtr.OptionalClassMethodsVMAddr)
		proto.OptionalClassMethods, err = f.GetObjCMethods(protoPtr.OptionalClassMethodsVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read optional class method vmaddr: %v", err)
		}
	}
	if protoPtr.InstancePropertiesVMAddr > 0 {
		protoPtr.InstancePropertiesVMAddr = f.vma.Convert(protoPtr.InstancePropertiesVMAddr)
		proto.InstanceProperties, err = f.GetObjCProperties(protoPtr.InstancePropertiesVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read instance property vmaddr: %v", err)
		}
	}
	if protoPtr.ExtendedMethodTypesVMAddr > 0 && protoPtr.HasExtendedMethodTypes() {
		protoPtr.ExtendedMethodTypesVMAddr = f.vma.Convert(protoPtr.ExtendedMethodTypesVMAddr)
		off, err := f.vma.GetOffset(protoPtr.ExtendedMethodTypesVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
		}

		extMPtr, err := f.ReadPointer(off)
		if err != nil {
			return nil, fmt.Errorf("failed to read ExtendedMethodTypesVMAddr: %v", err)
		}

		proto.ExtendedMethodTypes, err = f.GetCString(f.vma.Convert(extMPtr))
		if err != nil {
			return nil, fmt.Errorf("failed to read proto extended method types cstring: %v", err)
		}
	}
	if protoPtr.DemangledNameVMAddr > 0 && protoPtr.HasDemangledName() {
		protoPtr.DemangledNameVMAddr = f.vma.Convert(protoPtr.DemangledNameVMAddr)
		proto.DemangledName, err = f.GetCString(protoPtr.DemangledNameVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read proto demangled name cstring: %v", err)
		}
	}

	proto.ProtocolT = protoPtr

	return proto, nil
}

// GetObjCProtocols returns the Objective-C protocols
func (f *File) GetObjCProtocols() ([]objc.Protocol, error) {

	var protocols []objc.Protocol

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_protolist"); sec != nil {
				ptrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s.%s pointers: %v", sec.Seg, sec.Name, err)
				}

				for _, ptr := range ptrs {
					proto, err := f.getObjcProtocol(f.vma.Convert(ptr))
					if err != nil {
						return nil, fmt.Errorf("failed to read protocol at pointer %#x (converted %#x); %v", ptr, f.vma.Convert(ptr), err)
					}
					protocols = append(protocols, *proto)
				}
			}
		}
	}
	return protocols, nil
}

// GetObjCMethodList returns the Objective-C method list
func (f *File) GetObjCMethodList() ([]objc.Method, error) {
	var methodList objc.MethodList
	var objcMethods []objc.Method

	// TODO: test with 32bit / refactor
	if sec := f.Section("__TEXT", "__objc_methlist"); sec != nil {
		off, err := f.vma.GetOffset(f.vma.Convert(sec.Addr))
		if err != nil {
			return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
		}
		f.rr.Seek(int64(off), io.SeekStart)

		dat := make([]byte, sec.Size)
		if err := binary.Read(f.rr, f.ByteOrder, dat); err != nil {
			return nil, fmt.Errorf("failed to read %s.%s data: %v", sec.Seg, sec.Name, err)
		}

		r := bytes.NewReader(dat)

		for {
			err := binary.Read(r, f.ByteOrder, &methodList)

			currOffset, _ := r.Seek(0, io.SeekCurrent)
			currOffset += int64(sec.Offset)

			if err == io.EOF {
				break
			}

			if err != nil {
				return nil, fmt.Errorf("failed to read method_list_t: %v", err)
			}

			if methodList.IsSmall() {
				methods := make([]objc.MethodSmallT, methodList.Count)
				if err := binary.Read(r, f.ByteOrder, &methods); err != nil {
					return nil, fmt.Errorf("failed to read method_t(s) (small): %v", err)
				}
				for _, m := range methods {
					oMeth := objc.Method{}
					if f.Flags.DylibInCache() {
						if f.relativeSelectorBase > 0 {
							oMeth.NameVMAddr = f.relativeSelectorBase + uint64(m.NameOffset)
						} else {
							oMeth.NameVMAddr, err = f.vma.GetVMAddress(uint64(currOffset + int64(m.NameOffset)))
							if err != nil {
								return nil, fmt.Errorf("failed to convert offset %#x to vmaddr; %v", currOffset+int64(m.NameOffset), err)
							}
						}
					}
					oMeth.Name, err = f.GetCString(f.vma.Convert(oMeth.NameVMAddr))
					if err != nil {
						return nil, fmt.Errorf("failed to read method name cstring: %v", err)
					}
					oMeth.TypesVMAddr, err = f.vma.GetVMAddress(uint64(currOffset + 4 + int64(m.TypesOffset)))
					if err != nil {
						return nil, fmt.Errorf("failed to convert offset %#x to vmaddr; %v", currOffset+4+int64(m.TypesOffset), err)
					}
					oMeth.Types, err = f.GetCString(f.vma.Convert(oMeth.TypesVMAddr))
					if err != nil {
						return nil, fmt.Errorf("failed to read method types cstring: %v", err)
					}
					oMeth.ImpVMAddr, err = f.vma.GetVMAddress(uint64(currOffset + 8 + int64(m.ImpOffset)))
					if err != nil {
						return nil, fmt.Errorf("failed to convert offset %#x to vmaddr; %v", currOffset+8+int64(m.ImpOffset), err)
					}
					currOffset += int64(methodList.EntSize())
					objcMethods = append(objcMethods, oMeth)
				}
			} else {
				methods := make([]objc.MethodT, methodList.Count)
				f.rr.Seek(currOffset, io.SeekStart)
				if err := binaryReadStructs[method32T](f, methods); err != nil {
					return nil, fmt.Errorf("failed to read method_t(s) (small): %v", err)
				}
				for _, m := range methods {
					n, err := f.GetCString(f.vma.Convert(uint64(m.NameVMAddr)))
					if err != nil {
						return nil, fmt.Errorf("failed to read method name cstring: %v", err)
					}
					t, err := f.GetCString(f.vma.Convert(uint64(m.TypesVMAddr)))
					if err != nil {
						return nil, fmt.Errorf("failed to read method types cstring: %v", err)
					}
					if m.ImpVMAddr > 0 {
						_, err := f.vma.GetOffset(f.vma.Convert(m.ImpVMAddr))
						if err != nil {
							return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
						}
					}
					objcMethods = append(objcMethods, objc.Method{
						NameVMAddr:  m.NameVMAddr,
						TypesVMAddr: m.TypesVMAddr,
						ImpVMAddr:   m.ImpVMAddr,
						Name:        n,
						Types:       t,
					})
				}
			}
			// alignment
			curr, _ := r.Seek(0, io.SeekCurrent)
			align := types.RoundUp(uint64(curr), f.pointerSize())
			r.Seek(int64(align), io.SeekStart)
		}
	}

	return objcMethods, nil
}

// GetObjCMethods returns the Objective-C methods
func (f *File) GetObjCMethods(vmaddr uint64) ([]objc.Method, error) {

	var methodList objc.MethodList

	off, err := f.vma.GetOffset(f.vma.Convert(vmaddr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	if err := binary.Read(f.rr, f.ByteOrder, &methodList); err != nil {
		return nil, fmt.Errorf("failed to read method_list_t: %v", err)
	}

	if methodList.IsSmall() {
		return f.readSmallMethods(methodList)
	}

	return f.readBigMethods(methodList)
}

func (f *File) readSmallMethods(methodList objc.MethodList) (objcMethods []objc.Method, err error) {

	var nameVMAddr uint64

	currOffset, _ := f.rr.Seek(0, io.SeekCurrent)

	methods := make([]objc.MethodSmallT, methodList.Count)
	if err := binary.Read(f.rr, f.ByteOrder, &methods); err != nil {
		return nil, fmt.Errorf("failed to read method_t(s) (small): %v", err)
	}

	for _, method := range methods {
		if nameVMAddr, err = f.ReadPointer(uint64(currOffset) + uint64(method.NameOffset)); err != nil {
			return nil, fmt.Errorf("failed to read nameAddr(small): %v", err)
		}

		if f.Flags.DylibInCache() {
			if f.relativeSelectorBase > 0 {
				nameVMAddr = f.relativeSelectorBase + uint64(method.NameOffset)
			} else {
				nameVMAddr, err = f.vma.GetVMAddress(uint64(currOffset + int64(method.NameOffset)))
				if err != nil {
					return nil, fmt.Errorf("failed to convert offset %#x to vmaddr; %v", currOffset+int64(method.NameOffset), err)
				}
			}
		}

		n, err := f.GetCString(f.vma.Convert(nameVMAddr))
		if err != nil {
			return nil, fmt.Errorf("failed to read method name cstring: %v", err)
		}

		typesVMAddr, err := f.vma.GetVMAddress(uint64(currOffset + 4 + int64(method.TypesOffset)))
		if err != nil {
			return nil, fmt.Errorf("failed to convert offset %#x to vmaddr; %v", currOffset+4+int64(method.TypesOffset), err)
		}
		t, err := f.GetCString(f.vma.Convert(typesVMAddr))
		if err != nil {
			return nil, fmt.Errorf("failed to read method types cstring: %v", err)
		}

		impVMAddr, err := f.vma.GetVMAddress(uint64(currOffset + 8 + int64(method.ImpOffset)))
		if err != nil {
			return nil, fmt.Errorf("failed to convert offset %#x to vmaddr; %v", currOffset+8+int64(method.ImpOffset), err)
		}

		currOffset += int64(methodList.EntSize())

		objcMethods = append(objcMethods, objc.Method{
			NameVMAddr:  nameVMAddr,
			TypesVMAddr: typesVMAddr,
			ImpVMAddr:   impVMAddr,
			Name:        n,
			Types:       t,
		})
	}

	return objcMethods, nil
}

func (f *File) readBigMethods(methodList objc.MethodList) ([]objc.Method, error) {
	var objcMethods []objc.Method

	methods := make([]objc.MethodT, methodList.Count)
	if err := binaryReadStructs[method32T](f, methods); err != nil {
		return nil, fmt.Errorf("failed to read method_t: %v", err)
	}

	for _, method := range methods {
		n, err := f.GetCString(f.vma.Convert(uint64(method.NameVMAddr)))
		if err != nil {
			return nil, fmt.Errorf("failed to read method name cstring: %v", err)
		}
		t, err := f.GetCString(f.vma.Convert(uint64(method.TypesVMAddr)))
		if err != nil {
			return nil, fmt.Errorf("failed to read method types cstring: %v", err)
		}
		if method.ImpVMAddr > 0 {
			_, err := f.vma.GetOffset(f.vma.Convert(method.ImpVMAddr))
			if err != nil {
				return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
			}
		}
		objcMethods = append(objcMethods, objc.Method{
			NameVMAddr:  method.NameVMAddr,
			TypesVMAddr: method.TypesVMAddr,
			ImpVMAddr:   method.ImpVMAddr,
			Name:        n,
			Types:       t,
		})
	}

	return objcMethods, nil
}

// GetObjCIvars returns the Objective-C instance variables
func (f *File) GetObjCIvars(vmaddr uint64) ([]objc.Ivar, error) {

	var ivarsList objc.IvarList
	var ivars []objc.Ivar

	off, err := f.vma.GetOffset(f.vma.Convert(vmaddr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	if err := binary.Read(f.rr, f.ByteOrder, &ivarsList); err != nil {
		return nil, fmt.Errorf("failed to read objc_ivar_list_t: %v", err)
	}

	ivs := make([]objc.IvarT, ivarsList.Count)

	if err := binaryReadStructs[ivar32T](f, ivs); err != nil {
		return nil, fmt.Errorf("failed to read objc_ivar_list_t: %v", err)
	}

	for _, ivar := range ivs {
		ivar.Offset = f.vma.Convert(ivar.Offset)
		ivar.NameVMAddr = f.vma.Convert(ivar.NameVMAddr)
		ivar.TypesVMAddr = f.vma.Convert(ivar.TypesVMAddr)

		off, err := f.vma.GetOffset(ivar.Offset)
		if err != nil {
			return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
		}

		// offset is uintptr_t * (uint32 on 32 bit, uint64 on 64 bit)
		o, err := f.ReadPointer(off)
		if err != nil {
			return nil, fmt.Errorf("failed to read ivar.offset: %v", err)
		}
		n, err := f.GetCString(ivar.NameVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read ivar name cstring: %v", err)
		}
		t, err := f.GetCString(ivar.TypesVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read ivar types cstring: %v", err)
		}
		ivars = append(ivars, objc.Ivar{
			Name:   n,
			Type:   t,
			Offset: o,
			IvarT:  ivar,
		})
	}

	return ivars, nil
}

// GetObjCProperties returns the Objective-C properties
func (f *File) GetObjCProperties(vmaddr uint64) ([]objc.Property, error) {

	var propList objc.PropertyList
	var objcProperties []objc.Property

	off, err := f.vma.GetOffset(f.vma.Convert(vmaddr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	if err := binary.Read(f.rr, f.ByteOrder, &propList); err != nil {
		return nil, fmt.Errorf("failed to read objc_property_list_t: %v", err)
	}

	properties := make([]objc.PropertyT, propList.Count)

	if err := binaryReadStructs[property32T](f, properties); err != nil {
		return nil, fmt.Errorf("failed to read objc_property_t: %v", err)
	}

	for _, prop := range properties {
		prop.NameVMAddr = f.vma.Convert(prop.NameVMAddr)
		prop.AttributesVMAddr = f.vma.Convert(prop.AttributesVMAddr)

		name, err := f.GetCString(prop.NameVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read prop name cstring: %v", err)
		}
		attrib, err := f.GetCString(prop.AttributesVMAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to read prop attributes cstring: %v", err)
		}
		objcProperties = append(objcProperties, objc.Property{
			PropertyT:  prop,
			Name:       name,
			Attributes: attrib,
		})
	}

	return objcProperties, nil
}

// GetObjCClassReferences returns a map of classes to their section data virtual memory address
func (f *File) GetObjCClassReferences() (map[uint64]*objc.Class, error) {
	clsRefs := make(map[uint64]*objc.Class)

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_classrefs"); sec != nil { // External references to other classes
				classPtrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s.%s pointers: %v", sec.Seg, sec.Name, err)
				}

				for idx, ptr := range classPtrs {
					ptr = f.vma.Convert(ptr)
					if c, ok := f.objc[ptr]; ok {
						clsRefs[sec.Addr+uint64(idx*sizeOfInt64)] = c
					} else {
						if cls, err := f.GetObjCClass(ptr); err != nil {
							if f.HasFixups() {
								if bindName, err := f.GetBindName(ptr); err == nil {
									clsRefs[sec.Addr+uint64(idx*sizeOfInt64)] = &objc.Class{Name: strings.TrimPrefix(bindName, "_OBJC_CLASS_$_")}
								} else {
									return nil, fmt.Errorf("failed to read objc_class_t at classref ptr: %#x; %v", ptr, err)
								}
							}
							// TODO: don't swallow error here
						} else {
							clsRefs[sec.Addr+uint64(idx*sizeOfInt64)] = cls
							f.objc[ptr] = cls
						}
					}
				}
			}
		}
	}

	return clsRefs, nil
}

// GetObjCSuperReferences returns a map of super classes to their section data virtual memory address
func (f *File) GetObjCSuperReferences() (map[uint64]*objc.Class, error) {
	clsRefs := make(map[uint64]*objc.Class)

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_superrefs"); sec != nil { // External references to super classes
				classPtrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s.%s pointers: %v", sec.Seg, sec.Name, err)
				}

				for idx, ptr := range classPtrs {
					ptr = f.vma.Convert(ptr)
					if c, ok := f.objc[ptr]; ok {
						clsRefs[sec.Addr+uint64(idx*sizeOfInt64)] = c
					} else {
						if cls, err := f.GetObjCClass(ptr); err != nil {
							if f.HasFixups() {
								if bindName, err := f.GetBindName(ptr); err == nil {
									clsRefs[sec.Addr+uint64(idx*sizeOfInt64)] = &objc.Class{Name: strings.TrimPrefix(bindName, "_OBJC_CLASS_$_")}
								} else {
									return nil, fmt.Errorf("failed to read objc_class_t at superref ptr: %#x; %v", ptr, err)
								}
							}
							// TODO: don't swallow error here
						} else {
							clsRefs[sec.Addr+uint64(idx*sizeOfInt64)] = cls
							f.objc[ptr] = cls
						}
					}
				}
			}
		}
	}
	return clsRefs, nil
}

// GetObjCProtoReferences returns a map of protocol names to their section data virtual memory address
func (f *File) GetObjCProtoReferences() (map[uint64]*objc.Protocol, error) {
	protRefs := make(map[uint64]*objc.Protocol)

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			for _, secName := range []string{"__objc_protorefs", "__objc_protolist"} { // External references to protocols and list of ObjC protocols
				if sec := f.Section(s.Name, secName); sec != nil {
					protoPtrs, err := f.readPointersFromSection(sec)
					if err != nil {
						return nil, fmt.Errorf("failed to read %s.%s pointers: %v", sec.Seg, sec.Name, err)
					}

					for idx, ptr := range protoPtrs {
						proto, err := f.getObjcProtocol(f.vma.Convert(ptr))
						if err != nil {
							return nil, fmt.Errorf("failed to read objc_class_t at superref ptr: %#x (converted %#x); %v", ptr, f.vma.Convert(ptr), err)
						}
						protRefs[sec.Addr+uint64(idx*sizeOfInt64)] = proto
					}
				}
			}
		}
	}

	return protRefs, nil
}

// GetObjCSelectorReferences returns a map of selector names to their section data virtual memory address
func (f *File) GetObjCSelectorReferences() (map[uint64]*objc.Selector, error) {
	selRefs := make(map[uint64]*objc.Selector)

	for _, s := range f.Segments() {
		if strings.HasPrefix(s.Name, "__DATA") {
			if sec := f.Section(s.Name, "__objc_selrefs"); sec != nil { // External references to selectors
				selPtrs, err := f.readPointersFromSection(sec)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s.%s pointers: %v", sec.Seg, sec.Name, err)
				}

				for idx, sel := range selPtrs {
					sel = f.vma.Convert(sel)
					selName, err := f.GetCString(sel)
					if err != nil {
						return nil, fmt.Errorf("failed to read selector name cstring: %v", err)
					}
					selRefs[sec.Addr+uint64(idx*sizeOfInt64)] = &objc.Selector{
						VMAddr: sel,
						Name:   selName,
					}
				}
			}
		}
	}

	return selRefs, nil
}

// 32 bits helpers

type method32T objc.Method32T

func (m *method32T) CopyTo64(m64 *objc.MethodT) {
	m64.NameVMAddr = uint64(m.NameVMAddr)
	m64.TypesVMAddr = uint64(m.TypesVMAddr)
	m64.ImpVMAddr = uint64(m.ImpVMAddr)
}

type property32T objc.Property32T

func (p *property32T) CopyTo64(p64 *objc.PropertyT) {
	p64.NameVMAddr = uint64(p.NameVMAddr)
	p64.AttributesVMAddr = uint64(p.AttributesVMAddr)
}

type protocol32T objc.Protocol32T

func (p *protocol32T) CopyTo64(p64 *objc.ProtocolT) {
	p64.IsaVMAddr = uint64(p.IsaVMAddr)
	p64.NameVMAddr = uint64(p.NameVMAddr)
	p64.ProtocolsVMAddr = uint64(p.ProtocolsVMAddr)
	p64.InstanceMethodsVMAddr = uint64(p.InstanceMethodsVMAddr)
	p64.ClassMethodsVMAddr = uint64(p.ClassMethodsVMAddr)
	p64.OptionalInstanceMethodsVMAddr = uint64(p.OptionalInstanceMethodsVMAddr)
	p64.OptionalClassMethodsVMAddr = uint64(p.OptionalClassMethodsVMAddr)
	p64.InstancePropertiesVMAddr = uint64(p.InstancePropertiesVMAddr)
	p64.Size = p.Size
	p64.Flags = p.Flags
	p64.ExtendedMethodTypesVMAddr = uint64(p.ExtendedMethodTypesVMAddr)
	p64.DemangledNameVMAddr = uint64(p.DemangledNameVMAddr)
	p64.ClassPropertiesVMAddr = uint64(p.ClassPropertiesVMAddr)

	// convert Size to the value it would have on 64 bits
	// Size represent the size of the entire struct, signaling the presence of the last 3 fields.
	// all fields double in size fom 32 bits to 64 bits, except for Size and Flags ( 8 bytes )

	if p.Size >= 40 /* offsetof(Flags) + sizeof(Flags) => */ {
		p64.Size = 2*p.Size - 8
	} else {
		//FIXME: logger?
	}
}

type category32T objc.Category32T

func (c *category32T) CopyTo64(c64 *objc.CategoryT) {
	c64.NameVMAddr = uint64(c.NameVMAddr)
	c64.ClsVMAddr = uint64(c.ClsVMAddr)
	c64.InstanceMethodsVMAddr = uint64(c.InstanceMethodsVMAddr)
	c64.ClassMethodsVMAddr = uint64(c.ClassMethodsVMAddr)
	c64.ProtocolsVMAddr = uint64(c.ProtocolsVMAddr)
	c64.InstancePropertiesVMAddr = uint64(c.InstancePropertiesVMAddr)
}

type swiftClassMetadata objc.SwiftClassMetadata

func (m *swiftClassMetadata) CopyTo64(m64 *objc.SwiftClassMetadata64) {
	m64.IsaVMAddr = uint64(m.IsaVMAddr)
	m64.SuperclassVMAddr = uint64(m.SuperclassVMAddr)
	m64.MethodCacheBuckets = uint64(m.MethodCacheBuckets)
	m64.MethodCacheProperties = uint64(m.MethodCacheProperties)
	m64.DataVMAddrAndFastFlags = uint64(m.DataVMAddrAndFastFlags)
}

type classRO objc.ClassRO

func (c *classRO) CopyTo64(c64 *objc.ClassRO64) {
	c64.Flags = c.Flags
	c64.InstanceStart = c.InstanceStart
	c64.InstanceSize = uint64(c.InstanceSize)
	c64.IvarLayoutVMAddr = uint64(c.IvarLayoutVMAddr)
	c64.NameVMAddr = uint64(c.NameVMAddr)
	c64.BaseMethodsVMAddr = uint64(c.BaseMethodsVMAddr)
	c64.BaseProtocolsVMAddr = uint64(c.BaseProtocolsVMAddr)
	c64.IvarsVMAddr = uint64(c.IvarsVMAddr)
	c64.WeakIvarLayoutVMAddr = uint64(c.WeakIvarLayoutVMAddr)
	c64.BasePropertiesVMAddr = uint64(c.BasePropertiesVMAddr)
}

type ivar32T objc.Ivar32T

func (i *ivar32T) CopyTo64(i64 *objc.IvarT) {
	i64.Offset = uint64(i.Offset)
	i64.NameVMAddr = uint64(i.NameVMAddr)
	i64.TypesVMAddr = uint64(i.TypesVMAddr)
	i64.Alignment = i.Alignment
	i64.Size = i.Size
}

type cfstring32T objc.CFString32T

func (s *cfstring32T) CopyTo64(s64 *objc.CFString64T) {
	s64.IsaVMAddr = uint64(s.IsaVMAddr)
	s64.Info = uint64(s.Info)
	s64.Data = uint64(s.Data)
	s64.Length = uint64(s.Length)
}

type struct32Copier[Struct32 any, Struct64 any] interface {
	CopyTo64(target *Struct64)
	*Struct32 // force the implementation to be a pointer to Struct32
}

// NOTE: as methods cannot be generic, I opted to make all of these functions receive File as first parameter

// read a single pointer from current offset
func readPointer(f *File) (res uint64, err error) {
	if f.is64bit() {
		err = binary.Read(f.rr, f.ByteOrder, &res)
		return
	}
	var res32 uint32
	err = binary.Read(f.rr, f.ByteOrder, &res32)
	return uint64(res32), err
}

// read an array of pointers from current offset
func readPointers(f *File, count uint64) (res []uint64, err error) {
	res = make([]uint64, count)
	if f.is64bit() {
		err = binary.Read(f.rr, f.ByteOrder, res)
		return
	}
	buf := make([]byte, 4*count)
	if _, err = io.ReadFull(f.rr, buf); err != nil {
		return nil, err
	}

	for i := uint64(0); i < count; i++ {
		res[i] = uint64(f.ByteOrder.Uint32(buf[i*4 : i*4+4]))
	}
	return res, nil
}

// read a struct regardless of the pointerSize, from current offset
func binaryReadStruct[T32 any, T64 any, C struct32Copier[T32, T64]](f *File, target64 *T64) error {
	if f.is64bit() {
		return binary.Read(f.rr, f.ByteOrder, target64)
	}
	t32 := new(T32)

	if err := binary.Read(f.rr, f.ByteOrder, t32); err != nil {
		return err
	}

	C(t32).CopyTo64(target64)

	return nil
}

// read a slice of structs regardless of the pointerSize, from current offset
func binaryReadStructs[T32 any, T64 any, C struct32Copier[T32, T64]](f *File, target64 []T64) error {
	if f.is64bit() {
		return binary.Read(f.rr, f.ByteOrder, target64)
	}
	buf := make([]T32, len(target64))

	if err := binary.Read(f.rr, f.ByteOrder, buf); err != nil {
		return err
	}

	for i := range buf {
		t32 := &buf[i]
		C(t32).CopyTo64(&target64[i])
	}

	return nil
}

// read an indefinite number of structs regardless of the pointerSize, from the entire Section
func readStructsFromSection[T32 any, T64 any, C struct32Copier[T32, T64]](f *File, sec *Section) (res []T64, err error) {
	off, err := f.vma.GetOffset(f.vma.Convert(sec.Addr))
	if err != nil {
		return nil, fmt.Errorf("failed to convert vmaddr: %v", err)
	}
	f.rr.Seek(int64(off), io.SeekStart)

	dstSize := uint64(0)

	if f.is64bit() {
		dstSize = uint64(binary.Size(new(T64)))
	} else {
		dstSize = uint64(binary.Size(new(T32)))
	}

	nStructs := sec.Size / dstSize
	res = make([]T64, nStructs)
	if err := binaryReadStructs[T32, T64, C](f, res); err != nil {
		return nil, err
	}

	return
}
