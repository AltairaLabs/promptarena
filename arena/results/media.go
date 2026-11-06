package results

import "github.com/AltairaLabs/PromptKit/runtime/v2/types"

// MediaHasData reports whether a media part points at its bytes: inline data,
// a file path, a URL, or a storage reference. Media the engine externalized to
// <out>/media carries only the storage reference, and it loaded fine: counting
// it as a load error made a passing duplex run report audio errors.
func MediaHasData(media *types.MediaContent) bool {
	return nonEmpty(media.Data) || nonEmpty(media.FilePath) ||
		nonEmpty(media.URL) || nonEmpty(media.StorageReference)
}

// MediaSizeBytes returns the media's size: the inline data's length, else the
// recorded size_kb of externalized media, else 0.
func MediaSizeBytes(media *types.MediaContent) int64 {
	if nonEmpty(media.Data) {
		return int64(len(*media.Data))
	}
	if media.SizeKB != nil {
		return *media.SizeKB * bytesPerKB
	}
	return 0
}

const bytesPerKB = 1024

func nonEmpty(s *string) bool { return s != nil && *s != "" }
