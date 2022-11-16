package macho

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/blacktop/go-macho/types/objc"
)

type bindClassTest struct {
	targetClassName, resolvedSuperClassName string
}
type bindCategoryTest struct {
	targetCategoryName, resolvedClassName string
}

type objcFileTest struct {
	file       string
	classes    []*objc.Class
	protocols  []objc.Protocol
	categories []objc.Category

	classesSuperBinds    []bindClassTest
	categoriesClassBinds []bindCategoryTest
}

var fooProtocol = objc.Protocol{
	Name:                "FooProtocol",
	ExtendedMethodTypes: "v16@0:8",
	InstanceMethods: []objc.Method{
		{Name: "bar", Types: "*16@0:8"},
		{Name: "hello", Types: "v16@0:8"},
	},
	InstanceProperties: []objc.Property{
		{Name: "bar", Attributes: "T*,R"},
	},
	OptionalInstanceMethods: []objc.Method{
		{Name: "hallo", Types: "v16@0:8"},
	},
}

var fooClass = objc.Class{
	Name:       "Foo",
	SuperClass: "", // <ROOT>
	Prots:      []objc.Protocol{fooProtocol},
	Props: []objc.Property{
		{Name: "bar", Attributes: "T*,R,V_bar"},
	},
	Ivars: []objc.Ivar{
		{Name: "bar", Type: "*"},
	},
	InstanceMethods: []objc.Method{
		{Name: "hello", Types: "v16@0:8"},
		{Name: "bar", Types: "*16@0:8"},
	},
}

var fooDutchCategory = objc.Category{
	Name:  "Dutch",
	Class: nil, // resolves to "Foo" using binds
	InstanceMethods: []objc.Method{
		{Name: "hallo", Types: "v16@0:8"},
		{Name: "stroopwafel", Types: "v16@0:8"},
	},
}

var objcFileTests = []objcFileTest{
	{
		"internal/testdata/objc/class-gcc-amd64-darwin-exec.base64",
		[]*objc.Class{&fooClass},
		[]objc.Protocol{fooProtocol},
		[]objc.Category{fooDutchCategory},
		[]bindClassTest{{"Foo", "NSObject"}},
		[]bindCategoryTest{{"Dutch", "NSObject"}},
	},
}

func methodEquals(a, b *objc.Method) error {
	if a.Name != b.Name {
		return fmt.Errorf("different Name: want '%s', have '%s'", a.Name, b.Name)
	}
	if a.Types != b.Types {
		return fmt.Errorf("different Types: want '%s', have '%s'", a.Types, b.Types)
	}
	return nil
}

func propertyEquals(a, b *objc.Property) error {
	if a.Name != b.Name {
		return fmt.Errorf("different name: want '%s', have '%s'", a.Name, b.Name)
	}
	if a.Attributes != b.Attributes {
		return fmt.Errorf("different attributes: want '%s', have '%s'", a.Attributes, b.Attributes)
	}
	return nil
}

// I am too lazy, tests code quality / performance doesn't matter
var protocolMethods = map[string]func(i *objc.Protocol) []objc.Method{
	"instance methods":          func(i *objc.Protocol) []objc.Method { return i.InstanceMethods },
	"class methods":             func(i *objc.Protocol) []objc.Method { return i.ClassMethods },
	"optional instance methods": func(i *objc.Protocol) []objc.Method { return i.OptionalInstanceMethods },
	"optional class methods":    func(i *objc.Protocol) []objc.Method { return i.OptionalClassMethods },
}

func protocolEquals(a, b *objc.Protocol) error {
	if a == nil {
		if b == nil {
			return nil
		}
		return fmt.Errorf("expected null, have %v", b)
	} else if b == nil {
		return fmt.Errorf("expected class '%s', found null", a.Name)
	}

	if a.Name != b.Name {
		return fmt.Errorf("different name: want '%s', have '%s'", a.Name, b.Name)
	}
	if a.ExtendedMethodTypes != b.ExtendedMethodTypes {
		return fmt.Errorf("different extended method types: want '%s', have '%s'", a.ExtendedMethodTypes, b.ExtendedMethodTypes)
	}
	if a.DemangledName != b.DemangledName {
		return fmt.Errorf("different demangled name: want '%s', have '%s'", a.DemangledName, b.DemangledName)
	}

	if err := protocolsEquals(a.Prots, b.Prots); err != nil {
		return fmt.Errorf("different protocols: %v", err)
	}

	for name, fn := range protocolMethods {
		if err := methodsEquals(fn(a), fn(b)); err != nil {
			return fmt.Errorf("different %s: %v", name, err)
		}
	}

	if err := propertiesEquals(a.InstanceProperties, b.InstanceProperties); err != nil {
		return fmt.Errorf("different properties: %v", err)
	}

	return nil
}

func variableEquals(a, b *objc.Ivar) error {
	if a.Name != b.Name {
		return fmt.Errorf("different name: want '%s', have '%s'", a.Name, b.Name)
	}
	if a.Type != b.Type {
		return fmt.Errorf("different Type: want '%s', have '%s'", a.Type, b.Type)
	}
	return nil
}

func classEquals(a, b *objc.Class) error {
	if a == nil {
		if b == nil {
			return nil
		}
		return fmt.Errorf("expected null, have %v", b)
	} else if b == nil {
		return fmt.Errorf("expected class '%s', found null", a.Name)
	}
	if a.Name != b.Name {
		return fmt.Errorf("different name: want '%s', have '%s'", a.Name, b.Name)
	}
	if a.SuperClass != b.SuperClass {
		return fmt.Errorf("different SuperClass: want '%s', have '%s'", a.SuperClass, b.SuperClass)
	}
	if err := methodsEquals(a.ClassMethods, b.ClassMethods); err != nil {
		return fmt.Errorf("different class methods: %v", err)
	}
	if err := methodsEquals(a.InstanceMethods, b.InstanceMethods); err != nil {
		return fmt.Errorf("different instance mthod: %v", err)
	}
	if err := protocolsEquals(a.Prots, b.Prots); err != nil {
		return fmt.Errorf("different protocols: %v", err)
	}
	if err := propertiesEquals(a.Props, b.Props); err != nil {
		return fmt.Errorf("different properties: %v", err)
	}

	return nil
}

func categoryEquals(a, b *objc.Category) error {
	if a == nil {
		if b == nil {
			return nil
		}
		return fmt.Errorf("expected null, have %v", b)
	} else if b == nil {
		return fmt.Errorf("expected class '%s', found null", a.Name)
	}

	if a.Name != b.Name {
		return fmt.Errorf("different name: want '%s', have '%s'", a.Name, b.Name)
	}

	if err := classEquals(a.Class, b.Class); err != nil {
		return fmt.Errorf("different class: %v", err)
	}

	if err := protocolEquals(a.Protocol, b.Protocol); err != nil {
		return fmt.Errorf("different protocol: %v", err)
	}

	if err := propertiesEquals(a.Properties, b.Properties); err != nil {
		return fmt.Errorf("different properties: %v", err)
	}

	if err := methodsEquals(a.InstanceMethods, b.InstanceMethods); err != nil {
		return fmt.Errorf("different instance methods: %v", err)
	}

	if err := methodsEquals(a.ClassMethods, b.ClassMethods); err != nil {
		return fmt.Errorf("different class methods: %v", err)
	}

	return nil
}

func structsEquals[T any, F func(a, b *T) error](a, b []T, nameFn func(i *T) string, equalsFn F) error {
	if len(a) != len(b) {
		return fmt.Errorf("different amount of %T: want %d, have %d", *new(T), len(a), len(b))
	}
	if len(a) == 0 {
		return nil
	}
	lookup := make(map[string]*T)
	for i := range b {
		p := &b[i]
		lookup[nameFn(p)] = p
	}
	for i := range a {
		p := &a[i]
		name := nameFn(p)
		p1, found := lookup[name]
		if !found {
			return fmt.Errorf("%T not found: '%s'", *p, name)
		}
		if err := equalsFn(p, p1); err != nil {
			return fmt.Errorf("different %T '%s': %v", *p, name, err)
		}
	}
	return nil
}

func structPtrsEquals[T any, F func(a, b *T) error](a, b []*T, nameFn func(i *T) string, equalsFn F) error {
	if len(a) != len(b) {
		return fmt.Errorf("different amount of %T: want %d, have %d", *new(T), len(a), len(b))
	}
	if len(a) == 0 {
		return nil
	}
	lookup := make(map[string]*T)
	for i := range b {
		p := b[i]
		lookup[nameFn(p)] = p
	}
	for i := range a {
		p := a[i]
		name := nameFn(p)
		p1, found := lookup[name]
		if !found {
			return fmt.Errorf("%T not found: '%s'", *p, name)
		}
		if err := equalsFn(p, p1); err != nil {
			return fmt.Errorf("different %T '%s': %v", *p, name, err)
		}
	}
	return nil
}

func methodsEquals(a, b []objc.Method) error {
	return structsEquals(a, b, func(i *objc.Method) string { return i.Name }, methodEquals)
}

func propertiesEquals(a, b []objc.Property) error {
	return structsEquals(a, b, func(i *objc.Property) string { return i.Name }, propertyEquals)
}

func variablesEquals(a, b []objc.Ivar) error {
	return structsEquals(a, b, func(i *objc.Ivar) string { return i.Name }, variableEquals)
}

func protocolsEquals(a, b []objc.Protocol) error {
	return structsEquals(a, b, func(i *objc.Protocol) string { return i.Name }, protocolEquals)
}

func classesEquals(a, b []*objc.Class) error {
	return structPtrsEquals(a, b, func(i *objc.Class) string { return i.Name }, classEquals)
}

func categoriesEquals(a, b []objc.Category) error {
	return structsEquals(a, b, func(i *objc.Category) string { return i.Name }, categoryEquals)
}

func TestObjcStructs(t *testing.T) {
	var f *File
	var err error

	for _, expectations := range objcFileTests {
		if f, err = openObscured(expectations.file); err != nil {
			t.Fatalf("%s: failed to open test fixture: %v", expectations.file, err)
		}
		defer f.Close() // TODO: move out of the loop
		classes, err := f.GetObjCClasses()
		if err != nil {
			t.Fatalf("%s: failed to parse classes: %v", expectations.file, err)
		}
		if err := classesEquals(expectations.classes, classes); err != nil {
			t.Logf("want: %v\n\nhave: %v\n", expectations.classes, classes)
			t.Fatalf("%s: different classes: %v", expectations.file, err)
		}
		protocols, err := f.GetObjCProtocols()
		if err != nil {
			t.Fatalf("%s: failed to parse protocols: %v", expectations.file, err)
		}
		if err = protocolsEquals(expectations.protocols, protocols); err != nil {
			t.Logf("want: %v\n\nhave: %v\n", expectations.protocols, protocols)
			t.Fatalf("%s: different protocols: %v", expectations.file, err)
		}
		categories, err := f.GetObjCCategories()
		if err != nil {
			t.Fatalf("%s: failed to parse categories: %v", expectations.file, err)
		}
		if err := categoriesEquals(expectations.categories, categories); err != nil {
			t.Logf("want: %v\n\nhave: %v\n", expectations.categories, categories)
			t.Fatalf("%s: different categories: %v", expectations.file, err)
		}
	}
}

var _catT = objc.CategoryT{}

const categoryClassOffset = uint64(unsafe.Offsetof(_catT.ClsVMAddr))

var _clsT = objc.SwiftClassMetadata64{}

const clsSuperClassOffset = uint64(unsafe.Offsetof(_clsT.SuperclassVMAddr))

func TestObjcBinds(t *testing.T) {
	for _, expectations := range objcFileTests {
		f, err := openObscured(expectations.file)
		if err != nil {
			t.Fatalf("%s: failed to open file: %v", expectations.file, err)
		}
		defer f.Close() // TODO: move out of the loop
		binds, err := f.GetBindInfo()
		if err != nil {
			t.Fatalf("%s: failed to parse binds: %v", expectations.file, err)
		}
		bindsLookup := make(map[uint64]string, len(binds))
		for _, b := range binds {
			bindsLookup[b.Start+b.Offset] = strings.TrimPrefix(b.Name, "_OBJC_CLASS_$_")
		}
		classes, err := f.GetObjCClasses()
		if err != nil {
			t.Fatalf("%s: failed to parse classes: %v", expectations.file, err)
		}
		classLookup := make(map[string]*objc.Class, len(classes))
		for _, c := range classes {
			classLookup[c.Name] = c
		}
		categories, err := f.GetObjCCategories()
		if err != nil {
			t.Fatalf("%s: failed to parse categories: %v", expectations.file, err)
		}
		catLookup := make(map[string]*objc.Category, len(categories))
		for i := range categories {
			c := &categories[i]
			catLookup[c.Name] = c
		}
		// Classes' SuperClass
		for _, test := range expectations.classesSuperBinds {
			cls, found := classLookup[test.targetClassName]
			if !found {
				t.Fatalf("%s: class '%s' not found", expectations.file, test.targetClassName)
			}
			// the location whenre dyld shall place the pointer to the resolved super class
			superClassPtr := cls.ClassPtr + clsSuperClassOffset
			resolvedName, found := bindsLookup[superClassPtr]
			if !found {
				t.Fatalf("%s: class '%s' super class isn't in the list of binds", expectations.file, cls.Name)
			}
			if resolvedName != test.resolvedSuperClassName {
				t.Fatalf("%s: class '%s' super class resolved to a different name: want '%s', have '%s'",
					expectations.file, cls.Name, test.resolvedSuperClassName, resolvedName)
			}
		}
		// Categories' Class
		for _, test := range expectations.categoriesClassBinds {
			cat, found := catLookup[test.targetCategoryName]
			if !found {
				t.Fatalf("%s: category '%s' not found", expectations.file, test.targetCategoryName)
			}
			classPtr := cat.VMAddr + categoryClassOffset
			resolvedName, found := bindsLookup[classPtr]
			if !found {
				t.Fatalf("%s: category '%s' class isn't in the list of binds", expectations.file, cat.Name)
			}
			if resolvedName != test.resolvedClassName {
				t.Fatalf("%s: category '%s' class resolved to a different name: want '%s', have '%s'",
					expectations.file, cat.Name, test.resolvedClassName, resolvedName)
			}
		}
	}

}
