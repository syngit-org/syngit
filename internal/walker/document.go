package walker

import "bytes"

// ReplaceDocInContent replaces, within content, the first YAML document matching
// sel with newDoc and returns the merged content. When newDoc is empty the
// matching document is dropped (deletion). found is false (and content is
// returned unchanged) when no document matches; sibling documents are preserved
// verbatim.
func ReplaceDocInContent(content []byte, sel ObjectSelector, newDoc []byte) ([]byte, bool) {
	docs := bytes.Split(content, docSeparator)

	matched := -1
	for i, doc := range docs {
		if matchDoc(doc, sel) {
			matched = i
			break
		}
	}
	if matched == -1 {
		return content, false
	}

	if len(newDoc) == 0 {
		docs = append(docs[:matched], docs[matched+1:]...)
	} else {
		docs[matched] = bytes.TrimRight(newDoc, "\n")
	}

	merged := bytes.Join(docs, docSeparator)
	if len(merged) > 0 && !bytes.HasSuffix(merged, []byte("\n")) {
		merged = append(merged, '\n')
	}
	return merged, true
}

// appendDoc appends doc as a new YAML document at the end of existing, preserving
// the existing content.
func appendDoc(existing, doc []byte) []byte {
	out := bytes.TrimRight(existing, "\n")
	out = append(out, docSeparator...)
	out = append(out, doc...)
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return out
}
