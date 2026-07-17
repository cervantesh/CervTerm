package fontdesc

const (
	MaxPrimaryDescriptors           = 32
	MaxFallbackDescriptors          = 32
	MaxRules                        = 128
	MaxRangesPerRule                = 64
	MaxTotalRanges                  = 2048
	MaxEffectiveFeatures            = 64
	MaxDescriptorPayloadBytes       = 64 * 1024
	MaxDiscoveryFiles               = 20_000
	MaxDiscoveryFaces               = 65_536
	MaxFacesPerFile                 = 256
	MaxParsedFaces                  = 128
	MaxParsedBytes            int64 = 256 * 1024 * 1024
	MaxRetainedContexts             = 64
	MaxNegativeEntries              = 8_192
)
