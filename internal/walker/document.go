package walker

import (
	"bytes"
	"slices"
)

// DocTransform is given the repo-relative path of the file being written, the
// document already stored at that location (nil when there is none), and the
// document about to be written. It returns the bytes to write in its place.
//
// It is called at the only point where both the final path and the previous
// content of a document are known, which is what a content transformer such as
// the SOPS encryption needs: the path selects the .sops.yaml creation rule, and
// the previous document supplies the ciphertext to reuse.
//
// A nil DocTransform means "write the document unchanged".
type DocTransform func(relPath string, existing, content []byte) ([]byte, error)

// Apply runs the transform, treating a nil one as the identity.
func (t DocTransform) Apply(relPath string, existing, content []byte) ([]byte, error) {
	// A deletion carries no content and has nothing to transform.
	if t == nil || len(content) == 0 {
		return content, nil
	}
	return t(relPath, existing, content)
}

// ReplaceDocInContent replaces, within content, the first YAML document matching
// sel with newDoc and returns the merged content. When newDoc is empty the
// matching document is dropped (deletion). found is false (and content is
// returned unchanged) when no document matches; sibling documents are preserved
// verbatim.
func ReplaceDocInContent(content []byte, sel ObjectSelector, newDoc []byte) ([]byte, bool) {
	out, found, _ := ReplaceDocInContentFunc(content, sel, newDoc, "", nil)
	return out, found
}

// ReplaceDocInContentFunc is ReplaceDocInContent with a transform applied to
// newDoc before it is substituted. The transform receives the document being
// replaced as its existing content, so it sees exactly the bytes that this
// object currently occupies in the file rather than the whole file.
func ReplaceDocInContentFunc(content []byte, sel ObjectSelector, newDoc []byte, relPath string, transform DocTransform) ([]byte, bool, error) {
	docs := bytes.Split(content, docSeparator)

	matched := -1
	for i, doc := range docs {
		if matchDoc(doc, sel) {
			matched = i
			break
		}
	}
	if matched == -1 {
		return content, false, nil
	}

	if len(newDoc) == 0 {
		docs = append(docs[:matched], docs[matched+1:]...)
	} else {
		transformed, err := transform.Apply(relPath, docs[matched], newDoc)
		if err != nil {
			return content, true, err
		}
		docs[matched] = bytes.TrimRight(transformed, "\n")
	}

	merged := bytes.Join(docs, docSeparator)
	if len(merged) > 0 && !bytes.HasSuffix(merged, []byte("\n")) {
		merged = append(merged, '\n')
	}
	return merged, true, nil
}

// TransformChangedDocs is for content carrying the untouched documents of the
// file over verbatim: they may already be encrypted, so only the others are transformed.
func TransformChangedDocs(relPath string, existing, content []byte, transform DocTransform) ([]byte, error) {
	existingDocs := splitDocs(existing)

	docs := splitDocs(content)
	for i, doc := range docs {
		if slices.ContainsFunc(existingDocs, func(existingDoc []byte) bool { return bytes.Equal(existingDoc, doc) }) {
			continue
		}
		transformed, err := transform.Apply(relPath, previousDoc(existingDocs, doc), doc)
		if err != nil {
			return nil, err
		}
		docs[i] = bytes.TrimRight(transformed, "\n")
	}
	return append(bytes.Join(docs, docSeparator), '\n'), nil
}

func splitDocs(content []byte) [][]byte {
	var docs [][]byte
	for _, doc := range bytes.Split(content, docSeparator) {
		doc = bytes.TrimRight(bytes.TrimPrefix(doc, []byte("---\n")), "\n")
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, doc)
		}
	}
	return docs
}

// The namespace is ignored: a kustomize overlay may inject one the file does not hold.
func previousDoc(existingDocs [][]byte, doc []byte) []byte {
	sel := SelectorFromDoc(doc)
	for _, existingDoc := range existingDocs {
		existingSel := SelectorFromDoc(existingDoc)
		if existingSel.GVR == sel.GVR && existingSel.Name == sel.Name {
			return existingDoc
		}
	}
	return nil
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
