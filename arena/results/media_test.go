package results

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestMediaHasData_ExternalizedMediaIsLoaded(t *testing.T) {
	ref := "out/media/runs/r1/abc.wav"
	media := &types.MediaContent{StorageReference: &ref, MIMEType: "audio/pcm"}
	assert.True(t, MediaHasData(media), "externalized media carries only a storage reference")
}

func TestMediaHasData(t *testing.T) {
	s := "x"
	empty := ""
	assert.True(t, MediaHasData(&types.MediaContent{Data: &s}))
	assert.True(t, MediaHasData(&types.MediaContent{FilePath: &s}))
	assert.True(t, MediaHasData(&types.MediaContent{URL: &s}))
	assert.False(t, MediaHasData(&types.MediaContent{Data: &empty}))
	assert.False(t, MediaHasData(&types.MediaContent{}))
}

func TestMediaSizeBytes(t *testing.T) {
	data := "abcd"
	kb := int64(162)
	ref := "r"
	assert.Equal(t, int64(4), MediaSizeBytes(&types.MediaContent{Data: &data, SizeKB: &kb}))
	assert.Equal(t, int64(162*1024), MediaSizeBytes(&types.MediaContent{StorageReference: &ref, SizeKB: &kb}))
	assert.Equal(t, int64(0), MediaSizeBytes(&types.MediaContent{StorageReference: &ref}))
}
