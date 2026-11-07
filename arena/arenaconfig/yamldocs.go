package arenaconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// checkSingleDocument rejects a resource file holding more than one YAML
// document. Every loader reads only the first, so a second resource after a
// `---` separator used to be dropped without a word. A trailing separator or an
// empty document does not count. A file that does not parse is left to the
// loader, which reports the parse error.
func checkSingleDocument(data []byte, filename string) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	docs := 0
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil
		}
		if !isEmptyDocument(&node) {
			docs++
		}
	}
	if docs > 1 {
		return fmt.Errorf("%s holds %d YAML documents, but only one resource per file is read; "+
			"move each resource after a '---' separator into its own file and reference it from the arena config",
			filename, docs)
	}
	return nil
}

// isEmptyDocument reports a document with no content, or only a null.
func isEmptyDocument(node *yaml.Node) bool {
	if len(node.Content) == 0 {
		return true
	}
	root := node.Content[0]
	return root.Kind == yaml.ScalarNode && root.Tag == "!!null"
}
