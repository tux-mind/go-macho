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

var nsobjectProtocol = objc.Protocol{
	Name: "NSObject",
	InstanceMethods: []objc.Method{
		{Name: "isEqual:", Types: "B24@0:8@16"},
		{Name: "class", Types: "#16@0:8"},
		{Name: "self", Types: "@16@0:8"},
		{Name: "performSelector:", Types: "@24@0:8:16"},
		{Name: "performSelector:withObject:", Types: "@32@0:8:16@24"},
		{Name: "performSelector:withObject:withObject:", Types: "@40@0:8:16@24@32"},
		{Name: "isProxy", Types: "B16@0:8"},
		{Name: "isKindOfClass:", Types: "B24@0:8#16"},
		{Name: "isMemberOfClass:", Types: "B24@0:8#16"},
		{Name: "conformsToProtocol:", Types: "B24@0:8@16"},
		{Name: "respondsToSelector:", Types: "B24@0:8:16"},
		{Name: "retain", Types: "@16@0:8"},
		{Name: "release", Types: "Vv16@0:8"},
		{Name: "autorelease", Types: "@16@0:8"},
		{Name: "retainCount", Types: "Q16@0:8"},
		{Name: "zone", Types: "^{_NSZone=}16@0:8"},
		{Name: "hash", Types: "Q16@0:8"},
		{Name: "superclass", Types: "#16@0:8"},
		{Name: "description", Types: "@16@0:8"},
	},
	OptionalInstanceMethods: []objc.Method{
		{Name: "debugDescription", Types: "@16@0:8"},
	},
	InstanceProperties: []objc.Property{
		{Name: "hash", Attributes: "TQ,R"},
		{Name: "superclass", Attributes: "T#,R"},
		{Name: "description", Attributes: "T@\"NSString\",R,C"},
		{Name: "debugDescription", Attributes: "T@\"NSString\",R,C"},
	},
	ExtendedMethodTypes: "B24@0:8@16",
}

var uiAppDelegateProtocol = objc.Protocol{
	Name:                "UIApplicationDelegate",
	Prots:               []objc.Protocol{nsobjectProtocol},
	ExtendedMethodTypes: "v24@0:8@\"UIApplication\"16",
	OptionalInstanceMethods: []objc.Method{
		{Name: "applicationDidFinishLaunching:", Types: "v24@0:8@16"},
		{Name: "application:willFinishLaunchingWithOptions:", Types: "B32@0:8@16@24"},
		{Name: "application:didFinishLaunchingWithOptions:", Types: "B32@0:8@16@24"},
		{Name: "applicationDidBecomeActive:", Types: "v24@0:8@16"},
		{Name: "applicationWillResignActive:", Types: "v24@0:8@16"},
		{Name: "application:handleOpenURL:", Types: "B32@0:8@16@24"},
		{Name: "application:openURL:sourceApplication:annotation:", Types: "B48@0:8@16@24@32@40"},
		{Name: "application:openURL:options:", Types: "B40@0:8@16@24@32"},
		{Name: "applicationDidReceiveMemoryWarning:", Types: "v24@0:8@16"},
		{Name: "applicationWillTerminate:", Types: "v24@0:8@16"},
		{Name: "applicationSignificantTimeChange:", Types: "v24@0:8@16"},
		{Name: "application:willChangeStatusBarOrientation:duration:", Types: "v40@0:8@16q24d32"},
		{Name: "application:didChangeStatusBarOrientation:", Types: "v32@0:8@16q24"},
		{Name: "application:willChangeStatusBarFrame:", Types: "v56@0:8@16{CGRect={CGPoint=dd}{CGSize=dd}}24"},
		{Name: "application:didChangeStatusBarFrame:", Types: "v56@0:8@16{CGRect={CGPoint=dd}{CGSize=dd}}24"},
		{Name: "application:didRegisterUserNotificationSettings:", Types: "v32@0:8@16@24"},
		{Name: "application:didRegisterForRemoteNotificationsWithDeviceToken:", Types: "v32@0:8@16@24"},
		{Name: "application:didFailToRegisterForRemoteNotificationsWithError:", Types: "v32@0:8@16@24"},
		{Name: "application:didReceiveRemoteNotification:", Types: "v32@0:8@16@24"},
		{Name: "application:didReceiveLocalNotification:", Types: "v32@0:8@16@24"},
		{Name: "application:handleActionWithIdentifier:forLocalNotification:completionHandler:", Types: "v48@0:8@16@24@32@?40"},
		{Name: "application:handleActionWithIdentifier:forRemoteNotification:withResponseInfo:completionHandler:", Types: "v56@0:8@16@24@32@40@?48"},
		{Name: "application:handleActionWithIdentifier:forRemoteNotification:completionHandler:", Types: "v48@0:8@16@24@32@?40"},
		{Name: "application:handleActionWithIdentifier:forLocalNotification:withResponseInfo:completionHandler:", Types: "v56@0:8@16@24@32@40@?48"},
		{Name: "application:didReceiveRemoteNotification:fetchCompletionHandler:", Types: "v40@0:8@16@24@?32"},
		{Name: "application:performFetchWithCompletionHandler:", Types: "v32@0:8@16@?24"},
		{Name: "application:performActionForShortcutItem:completionHandler:", Types: "v40@0:8@16@24@?32"},
		{Name: "application:handleEventsForBackgroundURLSession:completionHandler:", Types: "v40@0:8@16@24@?32"},
		{Name: "application:handleWatchKitExtensionRequest:reply:", Types: "v40@0:8@16@24@?32"},
		{Name: "applicationShouldRequestHealthAuthorization:", Types: "v24@0:8@16"},
		{Name: "application:handlerForIntent:", Types: "@32@0:8@16@24"},
		{Name: "application:handleIntent:completionHandler:", Types: "v40@0:8@16@24@?32"},
		{Name: "applicationDidEnterBackground:", Types: "v24@0:8@16"},
		{Name: "applicationWillEnterForeground:", Types: "v24@0:8@16"},
		{Name: "applicationProtectedDataWillBecomeUnavailable:", Types: "v24@0:8@16"},
		{Name: "applicationProtectedDataDidBecomeAvailable:", Types: "v24@0:8@16"},
		{Name: "application:supportedInterfaceOrientationsForWindow:", Types: "Q32@0:8@16@24"},
		{Name: "application:shouldAllowExtensionPointIdentifier:", Types: "B32@0:8@16@24"},
		{Name: "application:viewControllerWithRestorationIdentifierPath:coder:", Types: "@40@0:8@16@24@32"},
		{Name: "application:shouldSaveSecureApplicationState:", Types: "B32@0:8@16@24"},
		{Name: "application:shouldRestoreSecureApplicationState:", Types: "B32@0:8@16@24"},
		{Name: "application:willEncodeRestorableStateWithCoder:", Types: "v32@0:8@16@24"},
		{Name: "application:didDecodeRestorableStateWithCoder:", Types: "v32@0:8@16@24"},
		{Name: "application:shouldSaveApplicationState:", Types: "B32@0:8@16@24"},
		{Name: "application:shouldRestoreApplicationState:", Types: "B32@0:8@16@24"},
		{Name: "application:willContinueUserActivityWithType:", Types: "B32@0:8@16@24"},
		{Name: "application:continueUserActivity:restorationHandler:", Types: "B40@0:8@16@24@?32"},
		{Name: "application:didFailToContinueUserActivityWithType:error:", Types: "v40@0:8@16@24@32"},
		{Name: "application:didUpdateUserActivity:", Types: "v32@0:8@16@24"},
		{Name: "application:userDidAcceptCloudKitShareWithMetadata:", Types: "v32@0:8@16@24"},
		{Name: "application:configurationForConnectingSceneSession:options:", Types: "@40@0:8@16@24@32"},
		{Name: "application:didDiscardSceneSessions:", Types: "v32@0:8@16@24"},
		{Name: "applicationShouldAutomaticallyLocalizeKeyCommands:", Types: "B24@0:8@16"},
		{Name: "window", Types: "@16@0:8"},
		{Name: "setWindow:", Types: "v24@0:8@16"},
	},
	InstanceProperties: []objc.Property{
		{Name: "window", Attributes: "T@\"UIWindow\",&,N"},
	},
}

var uiWindowSceneDelegateProtocol = objc.Protocol{
	Name: "UIWindowSceneDelegate",
	Prots: []objc.Protocol{{
		Name:  "UISceneDelegate",
		Prots: []objc.Protocol{nsobjectProtocol},
		OptionalInstanceMethods: []objc.Method{
			{Name: "scene:willConnectToSession:options:", Types: `v40@0:8@16@24@32`},
			{Name: "sceneDidDisconnect:", Types: `v24@0:8@16`},
			{Name: "sceneDidBecomeActive:", Types: `v24@0:8@16`},
			{Name: "sceneWillResignActive:", Types: `v24@0:8@16`},
			{Name: "sceneWillEnterForeground:", Types: `v24@0:8@16`},
			{Name: "sceneDidEnterBackground:", Types: `v24@0:8@16`},
			{Name: "scene:openURLContexts:", Types: `v32@0:8@16@24`},
			{Name: "stateRestorationActivityForScene:", Types: `@24@0:8@16`},
			{Name: "scene:restoreInteractionStateWithUserActivity:", Types: `v32@0:8@16@24`},
			{Name: "scene:willContinueUserActivityWithType:", Types: `v32@0:8@16@24`},
			{Name: "scene:continueUserActivity:", Types: `v32@0:8@16@24`},
			{Name: "scene:didFailToContinueUserActivityWithType:error:", Types: `v40@0:8@16@24@32`},
			{Name: "scene:didUpdateUserActivity:", Types: `v32@0:8@16@24`},
		},
		ExtendedMethodTypes: `v40@0:8@"UIScene"16@"UISceneSession"24@"UISceneConnectionOptions"32`,
	}},
	OptionalInstanceMethods: []objc.Method{
		{Name: "windowScene:didUpdateCoordinateSpace:interfaceOrientation:traitCollection:", Types: `v48@0:8@16@24q32@40`},
		{Name: "windowScene:performActionForShortcutItem:completionHandler:", Types: `v40@0:8@16@24@?32`},
		{Name: "windowScene:userDidAcceptCloudKitShareWithMetadata:", Types: `v32@0:8@16@24`},
		{Name: "window", Types: `@16@0:8`},
		{Name: "setWindow:", Types: `v24@0:8@16`},
	},
	InstanceProperties: []objc.Property{
		{Name: "window", Attributes: `T@"UIWindow",&,N`},
	},
	ExtendedMethodTypes: `v48@0:8@"UIWindowScene"16@"<UICoordinateSpace>"24q32@"UITraitCollection"40`,
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
	SuperClass: "", // NSOjbect
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

var liberrorClass = objc.Class{
	Name:            "SampleClass",
	SuperClass:      "", // NSObject
	Props:           []objc.Property{{Name: "classVersionNumber", Attributes: "Tq,R,N"}},
	InstanceMethods: []objc.Method{{Name: "classVersionNumber", Types: "q16@0:8"}},
}

var liberrorClass32 = objc.Class{
	Name:            "SampleClass",
	SuperClass:      "", // NSObject
	Props:           []objc.Property{{Name: "classVersionNumber", Attributes: "Ti,R,N"}},
	InstanceMethods: []objc.Method{{Name: "classVersionNumber", Types: "i8@0:4"}},
}

var objcFileTests = []objcFileTest{
	{
		file:                 "internal/testdata/objc/class-gcc-amd64-darwin-exec.base64",
		classes:              []*objc.Class{&fooClass},
		protocols:            []objc.Protocol{fooProtocol},
		categories:           []objc.Category{fooDutchCategory},
		classesSuperBinds:    []bindClassTest{{"Foo", "NSObject"}},
		categoriesClassBinds: []bindCategoryTest{{"Dutch", "NSObject"}},
	}, {
		file:              "internal/testdata/objc/liberror-arm64-darwin-dylib.base64",
		classes:           []*objc.Class{&liberrorClass},
		classesSuperBinds: []bindClassTest{{"SampleClass", "NSObject"}},
	}, {
		file:              "internal/testdata/objc/liberror-armv7-darwin-dylib.base64",
		classes:           []*objc.Class{&liberrorClass32},
		classesSuperBinds: []bindClassTest{{"SampleClass", "NSObject"}},
	}, {
		file:              "internal/testdata/objc/liberror-i386-darwin-dylib.base64",
		classes:           []*objc.Class{&liberrorClass32},
		classesSuperBinds: []bindClassTest{{"SampleClass", "NSObject"}},
	}, {
		file:              "internal/testdata/objc/liberror-x86_64-darwin-dylib.base64",
		classes:           []*objc.Class{&liberrorClass},
		classesSuperBinds: []bindClassTest{{"SampleClass", "NSObject"}},
	}, {
		// LC_DYLD_CHAINED_FIXUPS
		file: "internal/testdata/objc/breakmedaddy-armv8-darwin-exec.base64",
		classes: []*objc.Class{{
			Name:       "ViewController",
			SuperClass: "UIViewController",
			InstanceMethods: []objc.Method{
				{Name: "viewDidLoad", Types: "v16@0:8"},
				{Name: "isValidPin:", Types: "B24@0:8@16"},
				{Name: "tryMeButton:", Types: "v24@0:8@16"},
				{Name: "touchesBegan:withEvent:", Types: "v32@0:8@16@24"},
				{Name: "label", Types: "@16@0:8"},
				{Name: "setLabel:", Types: "v24@0:8@16"},
				{Name: "secret", Types: "@16@0:8"},
				{Name: "setSecret:", Types: "v24@0:8@16"},
				{Name: ".cxx_destruct", Types: "v16@0:8"},
			},
			Ivars: []objc.Ivar{
				{Name: "_secret", Type: "@\"UITextField\""},
				{Name: "_label", Type: "@\"UILabel\""},
			},
			Props: []objc.Property{
				{Name: "label", Attributes: "T@\"UILabel\",W,N,V_label"},
				{Name: "secret", Attributes: "T@\"UITextField\",W,N,V_secret"},
			},
		}, {
			Name:       "AppDelegate",
			SuperClass: "UIResponder",
			Prots:      []objc.Protocol{uiAppDelegateProtocol},
			InstanceMethods: []objc.Method{
				{Name: "application:didFinishLaunchingWithOptions:", Types: "B32@0:8@16@24"},
				{Name: "application:configurationForConnectingSceneSession:options:", Types: "@40@0:8@16@24@32"},
				{Name: "application:didDiscardSceneSessions:", Types: "v32@0:8@16@24"},
			},
			Props: []objc.Property{
				{Name: "window", Attributes: `T@"UIWindow",&,N`},
				{Name: "hash", Attributes: `TQ,R`},
				{Name: "superclass", Attributes: `T#,R`},
				{Name: "description", Attributes: `T@"NSString",R,C`},
				{Name: "debugDescription", Attributes: `T@"NSString",R,C`},
			},
		}, {
			Name:       "SceneDelegate",
			SuperClass: "UIResponder",
			Prots:      []objc.Protocol{uiWindowSceneDelegateProtocol},
			InstanceMethods: []objc.Method{
				{Name: "scene:willConnectToSession:options:", Types: "v40@0:8@16@24@32"},
				{Name: "sceneDidDisconnect:", Types: "v24@0:8@16"},
				{Name: "sceneDidBecomeActive:", Types: "v24@0:8@16"},
				{Name: "sceneWillResignActive:", Types: "v24@0:8@16"},
				{Name: "sceneWillEnterForeground:", Types: "v24@0:8@16"},
				{Name: "sceneDidEnterBackground:", Types: "v24@0:8@16"},
				{Name: "window", Types: "@16@0:8"},
				{Name: "setWindow:", Types: "v24@0:8@16"},
				{Name: ".cxx_destruct", Types: "v16@0:8"},
			},
			Props: []objc.Property{
				{Name: "window", Attributes: `T@"UIWindow",&,N,V_window`},
				{Name: "hash", Attributes: `TQ,R`},
				{Name: "superclass", Attributes: `T#,R`},
				{Name: "description", Attributes: `T@"NSString",R,C`},
				{Name: "debugDescription", Attributes: `T@"NSString",R,C`},
			},
		}},
		protocols: []objc.Protocol{
			nsobjectProtocol, uiWindowSceneDelegateProtocol, uiWindowSceneDelegateProtocol.Prots[0], uiAppDelegateProtocol,
		},
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

// I am too lazy
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
		if expectations.classes != nil {
			if err := classesEquals(expectations.classes, classes); err != nil {
				t.Logf("want: %v\n\nhave: %v\n", expectations.classes, classes)
				t.Fatalf("%s: different classes: %v", expectations.file, err)
			}
		}
		protocols, err := f.GetObjCProtocols()
		if err != nil {
			t.Fatalf("%s: failed to parse protocols: %v", expectations.file, err)
		}
		if expectations.protocols != nil {
			if err = protocolsEquals(expectations.protocols, protocols); err != nil {
				t.Logf("want: %v\n\nhave: %v\n", expectations.protocols, protocols)
				t.Fatalf("%s: different protocols: %v", expectations.file, err)
			}
		}
		categories, err := f.GetObjCCategories()
		if err != nil {
			t.Fatalf("%s: failed to parse categories: %v", expectations.file, err)
		}
		if expectations.categories != nil {
			if err := categoriesEquals(expectations.categories, categories); err != nil {
				t.Logf("want: %v\n\nhave: %v\n", expectations.categories, categories)
				t.Fatalf("%s: different categories: %v", expectations.file, err)
			}
		}
	}
}

var _catT = objc.CategoryT{}
var _cat32T = objc.CategoryT{}

const categoryClassOffset = uint64(unsafe.Offsetof(_catT.ClsVMAddr))
const categoryClassOffset32 = uint64(unsafe.Offsetof(_cat32T.ClsVMAddr))

var _clsT = objc.SwiftClassMetadata64{}
var _cls32T = objc.SwiftClassMetadata{}

const clsSuperClassOffset = uint64(unsafe.Offsetof(_clsT.SuperclassVMAddr))
const clsSuperClassOffset32 = uint64(unsafe.Offsetof(_cls32T.SuperclassVMAddr))

func TestObjcBinds(t *testing.T) {
	for _, expectations := range objcFileTests {
		if expectations.classesSuperBinds == nil && expectations.categoriesClassBinds == nil {
			continue
		}
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
		clsSuperOffset := clsSuperClassOffset
		catClassOffset := categoryClassOffset
		if !f.is64bit() {
			clsSuperOffset = clsSuperClassOffset32
			catClassOffset = categoryClassOffset32
		}
		// Classes' SuperClass
		for _, test := range expectations.classesSuperBinds {
			cls, found := classLookup[test.targetClassName]
			if !found {
				t.Fatalf("%s: class '%s' not found", expectations.file, test.targetClassName)
			}
			// the location whenre dyld shall place the pointer to the resolved super class
			superClassPtr := cls.ClassPtr + clsSuperOffset
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
			classPtr := cat.VMAddr + catClassOffset
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
