#include <stdio.h>
#import <Foundation/Foundation.h>

@protocol FooProtocol

@property (readonly) char *bar;

-(void)hello;
@optional
-(void)hallo;

@end

@interface Foo: NSObject <FooProtocol> {
    char * _bar;
}
@property (readonly) char *bar;
-(void)hello;
@end

@implementation Foo
@synthesize bar = _bar;
-(void)hello {
    printf("Hello world!");
}
@end

@interface NSObject (Dutch)
-(void)hallo;
@end

@implementation NSObject (Dutch)
-(void)hallo {
    printf("Hallo Wereld");
}
-(void)stroopwafel {
    printf("sweet!");
}
@end

int main(){
    Foo *foo = [Foo alloc];
    [foo hello];
    [foo dealloc];

    return 0;
}