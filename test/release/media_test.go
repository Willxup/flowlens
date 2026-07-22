package release_test

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReadmeScreenshotsHaveMatchingDimensionsAndNoMetadata(t *testing.T) {
	paths := []string{"assets/flowlens-light.png", "assets/flowlens-dark.png"}
	var width, height uint32
	for index, path := range paths {
		contents := []byte(readRepositoryFile(t, path))
		gotWidth, gotHeight := inspectPNG(t, path, contents)
		if gotWidth == 0 || gotHeight == 0 {
			t.Fatalf("%s dimensions = %dx%d", path, gotWidth, gotHeight)
		}
		if index == 0 {
			width, height = gotWidth, gotHeight
			continue
		}
		if gotWidth != width || gotHeight != height {
			t.Fatalf("%s dimensions = %dx%d, want %dx%d", path, gotWidth, gotHeight, width, height)
		}
	}
}

func inspectPNG(t *testing.T, path string, contents []byte) (uint32, uint32) {
	t.Helper()
	signature := []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'}
	if len(contents) < len(signature) || !bytes.Equal(contents[:len(signature)], signature) {
		t.Fatalf("%s is not a PNG", path)
	}
	forbidden := map[string]bool{"tEXt": true, "zTXt": true, "iTXt": true, "eXIf": true}
	var width, height uint32
	for offset := len(signature); offset+12 <= len(contents); {
		length := int(binary.BigEndian.Uint32(contents[offset : offset+4]))
		end := offset + 12 + length
		if length < 0 || end > len(contents) {
			t.Fatalf("%s has an invalid PNG chunk", path)
		}
		kind := string(contents[offset+4 : offset+8])
		if forbidden[kind] {
			t.Errorf("%s contains forbidden %s metadata", path, kind)
		}
		if kind == "IHDR" {
			if length != 13 {
				t.Fatalf("%s has an invalid IHDR", path)
			}
			width = binary.BigEndian.Uint32(contents[offset+8 : offset+12])
			height = binary.BigEndian.Uint32(contents[offset+12 : offset+16])
		}
		offset = end
		if kind == "IEND" {
			return width, height
		}
	}
	t.Fatalf("%s has no IEND chunk", path)
	return 0, 0
}
