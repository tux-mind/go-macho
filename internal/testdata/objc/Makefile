class-gcc-amd64-darwin-exec.base64: class.m
	gcc $^ -arch x86_64 -framework Foundation -o /dev/stdout -l objc | base64 > $@