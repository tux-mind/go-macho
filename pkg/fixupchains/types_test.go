package fixupchains

// interface assertions
var _rebaseTypes = []Rebase{
	// 32 bits
	&DyldChainedPtr32Rebase{},
	&DyldChainedPtr32CacheRebase{},
	&DyldChainedPtr32FirmwareRebase{},
	// 64 bits
	&DyldChainedPtr64Rebase{},
	&DyldChainedPtr64RebaseOffset{},
	&DyldChainedPtr64KernelCacheRebase{},
	&DyldChainedPtrArm64eRebase{},
	&DyldChainedPtrArm64eRebase24{},
	// 64 bits authenticated
	&DyldChainedPtrArm64eAuthRebase{},
	&DyldChainedPtrArm64eAuthRebase24{},
}
