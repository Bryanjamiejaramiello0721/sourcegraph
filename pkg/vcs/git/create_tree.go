package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/sourcegraph/sourcegraph/pkg/gitserver"
)

const (
	SHAAllZeros  = "0000000000000000000000000000000000000000"
	SHAEmptyBlob = "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"
)

// Tree represents a git tree (i.e., "git ls-tree -r -d").
type Tree struct {
	byPath map[string]*TreeEntry // key is path without leading or trailing slashes (e.g., "d", "d1/d2", "d/f")
	Root   []*TreeEntry
}

func (t *Tree) Add(path string, e *TreeEntry) error {
	if path == "" || path == "." {
		panic(fmt.Sprintf("bad path: %q", path))
	}

	if t.byPath == nil {
		t.byPath = map[string]*TreeEntry{}
	}
	if _, exists := t.byPath[path]; exists {
		return fmt.Errorf("Tree.Add: path already exists: %q", path)
	}
	t.byPath[path] = e

	dir := filepath.Dir(path)
	if dir == "." {
		t.Root = append(t.Root, e)
	} else {
		parent := t.byPath[dir]
		if parent == nil {
			panic(fmt.Sprintf("no parent tree for %q", path))
		}
		parent.Entries = append(parent.Entries, e)
	}
	return nil
}

// remove removes path from the tree. It does not prune trees that are
// empty after the removal.
func (t *Tree) remove(path string) {
	var parentEntries *[]*TreeEntry
	if dir := filepath.Dir(path); dir == "." {
		parentEntries = &t.Root
	} else {
		parentEntries = &t.byPath[dir].Entries
	}

	name := filepath.Base(path)
	for i, e := range *parentEntries {
		if e.Name == name {
			a := *parentEntries

			// Delete item without causing memory leak (see
			// https://github.com/golang/go/wiki/SliceTricks).
			copy(a[i:], a[i+1:])
			a[len(a)-1] = nil
			a = a[:len(a)-1]

			*parentEntries = a
			break
		}
	}
}

func (t *Tree) Get(path string) *TreeEntry {
	return t.byPath[path]
}

// createOrDirtyAncestors creates all ancestor trees of path (if they
// don't already exist). If any ancestor trees already exist, it marks
// them as having changed. Their oids are zeroed out and need to be
// recomputed later.
func (t *Tree) createOrDirtyAncestors(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}

	comps := strings.Split(dir, string(os.PathSeparator))
	var p string // ancestor path
	for i, c := range comps {
		if i == 0 {
			p = c
		} else {
			p += string(os.PathSeparator) + c
		}

		ancestor, present := t.byPath[p]
		if !present {
			// Create ancestor's entry in its parent if it doesn't yet
			// exist.
			ancestor = &TreeEntry{
				Mode: "040000",
				Type: "tree",
				Name: filepath.Base(p),
			}
			if err := t.Add(p, ancestor); err != nil {
				return err
			}
		}
		ancestor.OID = "" // mark dirty
	}
	return nil
}

func (t *Tree) ApplyChanges(changes []*ChangedFile) error {
	for _, f := range changes {
		status := f.Status[0] // see "git diff-index --help" RAW OUTPUT FORMAT section for values

		if status == 'M' { // M=in-place edit
			src := t.Get(f.SrcPath)
			src.Mode = f.DstMode
			src.OID = f.DstSHA
			if err := t.createOrDirtyAncestors(f.SrcPath); err != nil {
				return err
			}
		}

		if status == 'A' || status == 'C' || status == 'R' { // A=create, C=copy-edit, R=rename-edit
			var path string
			if status == 'A' {
				path = f.SrcPath // "git diff-index" calls created files' paths their src path not dst path
			} else {
				path = f.DstPath
			}

			typ, err := objectTypeForMode(f.DstMode)
			if err != nil {
				return err
			}

			e := &TreeEntry{
				Mode: f.DstMode,
				Type: typ,
				OID:  f.DstSHA,
				Name: filepath.Base(path),
			}
			if err := t.createOrDirtyAncestors(path); err != nil {
				return err
			}
			if err := t.Add(path, e); err != nil {
				return err
			}
		}

		if status == 'D' || status == 'R' { // D=delete, R=rename-edit
			if err := t.createOrDirtyAncestors(f.SrcPath); err != nil {
				return err
			}
			t.remove(f.SrcPath)
		}
	}
	return nil
}

type TreeEntry struct {
	Mode    string
	Type    string // object type (blob, tree, etc.)
	OID     string
	Name    string
	Entries []*TreeEntry
}

func (e TreeEntry) String() string {
	s := fmt.Sprintf("%s %s %s %s", e.Mode, e.Type, e.OID, e.Name)
	if len(e.Entries) > 0 {
		entryNames := make([]string, len(e.Entries))
		for i, c := range e.Entries {
			entryNames[i] = c.Name
		}
		s += fmt.Sprintf(" [children: %s]", strings.Join(entryNames, " "))
	}
	return s
}

// ChangedFile represents a line in "git diff-index" output.
type ChangedFile struct {
	Status           string
	SrcMode, DstMode string
	SrcSHA, DstSHA   string
	SrcPath, DstPath string
}

func (c *ChangedFile) String() string {
	switch c.Status {
	case "A":
		return fmt.Sprintf("add %s", c.SrcPath)
	case "C":
		return fmt.Sprintf("copy %s -> %s", c.SrcPath, c.DstPath)
	case "D":
		return fmt.Sprintf("delete %s", c.SrcPath)
	case "M":
		return fmt.Sprintf("mod %s", c.SrcPath)
	case "R":
		return fmt.Sprintf("rename %s -> %s", c.SrcPath, c.DstPath)
	case "T":
		return fmt.Sprintf("change type %s %s -> %s", c.SrcPath, c.SrcMode, c.DstMode)
	case "U":
		return fmt.Sprintf("unmerged %s", c.SrcPath)
	default:
		return fmt.Sprintf("%s %s", c.Status, c.SrcPath)
	}
}

const RegularFileNonExecutableMode = "100644"

func objectTypeForMode(modeStr string) (string, error) {
	switch modeStr {
	case "040000": // directory
		return "tree", nil
	case RegularFileNonExecutableMode, "100755": // regular file
		return "blob", nil
	case "120000": // symlink
		return "blob", nil
	case "160000": // submodule
		return "commit", nil
	default:
		return "", fmt.Errorf("unrecognized git mode %q", modeStr)
	}
}

func CreateTree(ctx context.Context, repo gitserver.Repo, base string, ops TODO) (string, error) {
	tree, err := listTreeFull(ctx, repo, base)
	if err != nil {
		return "", err
	}

	updateGitFile := func(filename string, newData []byte) error {
		if strings.HasPrefix(filename, "/") || strings.HasPrefix(filename, "#") {
			panic(fmt.Sprintf("expected stripped filename, got %q", filename))
		}
		e := tree.Get(filename)
		newOID, err := hashObject(ctx, repo, "blob", filename, newData)
		if err != nil {
			return err
		}
		return tree.ApplyChanges([]*ChangedFile{{
			Status:  "M",
			SrcMode: e.Mode, DstMode: e.Mode,
			SrcSHA: e.OID, DstSHA: newOID,
			SrcPath: filename,
		}})
	}

	created := make(map[string]struct{})
	originatedFrom := make(map[string]string)
	for _, iop := range ops {
		switch op := iop.(type) {
		case ot.FileCopy:
			src, dst := op.Src, op.Dst
			if isBufferPath(src) && isFilePath(dst) && stripFileOrBufferPath(src) == stripFileOrBufferPath(dst) {
				// Save
				data, err := fbuf.ReadFile(stripFileOrBufferPath(src))
				if err != nil {
					return "", err
				}
				if err := updateGitFile(stripFileOrBufferPath(src), data); err != nil {
					return "", err
				}
				if err := fbuf.Remove(stripFileOrBufferPath(src)); err != nil {
					return "", err
				}
			} else if isBufferPath(src) {
				panic("not yet implemented")
			}
			if isBufferPath(dst) {
				data, _, _, err := gitRepo.ReadBlob(base, stripFileOrBufferPath(src))
				if err != nil {
					return "", err
				}
				if err := fbuf.Exists(stripFileOrBufferPath(dst)); !os.IsNotExist(err) {
					err = fmt.Errorf("copy %q to %q: destination file %q already exists", src, dst, dst)
					if panicOnSomeErrors {
						panic(err)
					}
					return "", err
				}
				if err := fbuf.WriteFile(stripFileOrBufferPath(dst), data, 0666); err != nil {
					return "", err
				}
			} else {
				// TODO(sqs): handle copy-then-modify
				mode, sha, err := gitRepo.FileInfoForPath(base, stripFileOrBufferPath(src))
				if err != nil {
					return "", err
				}
				if err := tree.ApplyChanges([]*ChangedFile{{
					Status:  "C",
					SrcMode: mode, DstMode: mode,
					SrcSHA: sha, DstSHA: sha,
					SrcPath: stripFileOrBufferPath(src), DstPath: stripFileOrBufferPath(dst),
				}}); err != nil {
					level.Error(logger).Log("tree-apply-changes-failed", err, "src", src, "dst", dst, "base", base, "op", op)
					return "", err
				}
			}
			originatedFrom[dst] = src
		case ot.FileRename:
			src, dst := op.Src, op.Dst
			if isBufferPath(src) || isBufferPath(dst) {
				panic("not yet implemented")
			}

			// TODO(sqs): handle rename-then-modify
			mode, sha, err := gitRepo.FileInfoForPath(base, stripFileOrBufferPath(src))
			if err != nil {
				return "", err
			}
			if err := tree.ApplyChanges([]*ChangedFile{{
				Status:  "R",
				SrcMode: mode, DstMode: mode,
				SrcSHA: sha, DstSHA: sha,
				SrcPath: stripFileOrBufferPath(src), DstPath: stripFileOrBufferPath(dst),
			}}); err != nil {
				return "", err
			}
			originatedFrom[dst] = src
		case ot.FileCreate:
			f := op.File
			created[f] = struct{}{}

			if isBufferPath(f) {
				if err := fbuf.WriteFile(stripFileOrBufferPath(f), nil, 0666); err != nil {
					return "", err
				}
			} else {
				// Ensure we have the object for the empty blob.
				//
				// NOTE: This *almost* always produces the
				// SHAEmptyBlob oid, but you can hack gitattributes to
				// make this return something else, and we want to avoid
				// making assumptions about your git repo that could ever be
				// violated.
				//
				// TODO(sqs): could optimize by leaving the dstSHA blank for
				// newly created files that we have nonzero edits for (we will
				// compute the dstSHA again for those files anyway).
				oid, err := gitRepo.HashObject("blob", stripFileOrBufferPath(f), nil)
				if err != nil {
					return "", err
				}

				// We will fill in the dstSHA below in Edit when we have the
				// file contents, if we have edits.
				if err := tree.ApplyChanges([]*ChangedFile{{
					Status:  "A",
					SrcMode: SHAAllZeros, DstMode: RegularFileNonExecutableMode,
					SrcSHA: SHAAllZeros, DstSHA: oid,
					SrcPath: stripFileOrBufferPath(f),
				}}); err != nil {
					return "", err
				}
			}
		case ot.FileDelete:
			f := op.File
			if isBufferPath(f) {
				if err := fbuf.Remove(stripFileOrBufferPath(f)); err != nil {
					return "", err
				}
			} else {
				mode, sha, err := gitRepo.FileInfoForPath(base, stripFileOrBufferPath(f))
				if err != nil {
					return "", err
				}
				if err := tree.ApplyChanges([]*ChangedFile{{
					Status:  "D",
					SrcMode: mode, DstMode: SHAAllZeros,
					SrcSHA: sha, DstSHA: SHAAllZeros,
					SrcPath: stripFileOrBufferPath(f),
				}}); err != nil {
					return "", err
				}
			}
		case ot.FileTruncate:
			f := op.File
			if isBufferPath(f) {
				if err := fbuf.WriteFile(stripFileOrBufferPath(f), nil, 0666); err != nil {
					return "", err
				}
			} else {
				if err := updateGitFile(stripFileOrBufferPath(f), nil); err != nil {
					return "", err
				}
			}
		case ot.FileEdit:
			f, edits := op.File, op.Edits
			if len(edits) == 0 {
				continue
			}

			var data []byte
			var err error
			if _, created := created[f]; created {
				// no data yet
				data = []byte{}
			} else if isBufferPath(f) {
				data, err = fbuf.ReadFile(stripFileOrBufferPath(f))
			} else {
				f0, ok := originatedFrom[f]
				if ok && !isFilePath(f0) {
					panic(fmt.Sprintf("not implemented: edit of a disk file %q derived from buffer file %q", f, f0))
				}
				if !ok {
					f0 = f
				}
				data, _, _, err = gitRepo.ReadBlob(base, stripFileOrBufferPath(f0))
			}
			if err != nil {
				return "", err
			}

			doc := ot.Doc(string(data))
			if err := doc.Apply(edits); err != nil {
				err := zap.Errorf(zap.ErrorCodeInvalidOp, "apply OT edit to %s @ %s: %s (doc: %q, op: %v)", f, base, err, data, op)
				if panicOnSomeErrors {
					level.Error(logger).Log("PANIC-BELOW", "")
					panic(err)
				}
				return "", err
			}

			if isBufferPath(f) {
				if err := fbuf.WriteFile(stripFileOrBufferPath(f), []byte(string(doc)), 0666); err != nil {
					return "", err
				}
			} else {
				if err := updateGitFile(stripFileOrBufferPath(f), []byte(string(doc))); err != nil {
					return "", err
				}
			}
		}
	}
	if tree == nil {
		return "", nil // indicates no new tree SHA was created
	}
	return gitRepo.CreateTree("", tree.Root)
}

func listTreeFull(ctx context.Context, repo gitserver.Repo, head string) (*Tree, error) {
	if err := checkSpecArgSafety(head); err != nil {
		return nil, err
	}

	// This is pretty fast, even on large repositories (45ms on the
	// Sourcegraph repository).
	cmd := gitserver.DefaultClient.Command("git", "ls-tree", "-r", "-t", "-z", "--full-tree", "--full-name", head)
	cmd.Repo = repo
	out, err := cmd.Output(ctx)
	if err != nil {
		return nil, err
	}
	var t Tree
	for _, line := range splitNullsBytes(out) {
		mode, typ, oid, path, err := parseLsTreeLine(line)
		if err != nil {
			return nil, err
		}
		t.Add(path, &TreeEntry{
			Mode: mode,
			Type: typ,
			OID:  oid,
			Name: filepath.Base(path),
		})
	}
	return &t, nil
}

func readBlob(ctx context.Context, treeish, path string) (data []byte, mode, oid string, err error) {
	mode, oid, err = fileInfoForPath(ctx, treeish, path)
	if err != nil {
		return nil, "", "", err
	}
	typ, err := objectTypeForMode(mode)
	if err != nil || typ != "blob" {
		return nil, "", "", &os.PathError{Op: "gitReadBlob (tree: " + treeish + ")", Path: path, Err: os.ErrInvalid}
	}

	if err := checkSpecArgSafety(treeish); err != nil {
		return nil, "", "", err
	}
	if strings.Contains(treeish, ":") {
		return nil, "", "", fmt.Errorf("bad treeish arg (contains ':'): %q", treeish)
	}
	if err := checkSpecArgSafety(path); err != nil {
		return nil, "", "", err
	}
	contents, err := r.Exec(nil, "cat-file", typ, oid)
	if err != nil {
		return nil, "", "", err
	}
	return contents, mode, oid, nil
}

func fileInfoForPath(ctx context.Context, treeish, path string) (mode, oid string, err error) {
	out, err := r.Exec(nil, "ls-tree", "-z", "-t", "--full-name", "--full-tree", "--", treeish, path)
	if err != nil {
		return "", "", err
	}
	for _, line := range splitNullsBytes(out) {
		mode, _, oid, path2, err := parseLsTreeLine(line)
		if err != nil {
			return "", "", err
		}
		if path2 == path {
			return mode, oid, nil
		}
	}
	return "", "", &os.PathError{Op: "gitFileInfoForPath (tree: " + treeish + ")", Path: path, Err: os.ErrNotExist}
}

func createTree(ctx context.Context, basePath string, entries []*TreeEntry) (string, error) {
	var buf bytes.Buffer // output in the "git ls-tree" format
	for _, e := range entries {
		// Entries that were added, and the ancestor trees thereof,
		// have empty or all-zero SHAs.
		if e.OID == "" || e.OID == SHAAllZeros {
			path := filepath.Join(basePath, e.Name)

			switch e.Type {
			case "blob":
				return "", fmt.Errorf("tree entry blob at %q must have OID set when creating tree in bare repo (OID is %q)", path, e.OID)

			case "tree":
				var err error
				e.OID, err = r.CreateTree(path, e.Entries)
				if err != nil {
					return "", err
				}

			default:
				// There are no known cases that this should happen
				// for, but this case is handled with an error message
				// just in case.
				//
				// This is only triggered when e.oid is zeroed out,
				// which should never happen for submodules (e.typ ==
				// "commit").
				return "", fmt.Errorf("repository contains unsupported tree entry type %q at %q", e.Type, path)
			}
		}

		fmt.Fprintf(&buf, "%s %s %s\t%s\x00", e.Mode, e.Type, e.OID, e.Name)
	}

	const commitVerbose = false // DEV

	stdinBytes := buf.Bytes()
	oidBytes, err := r.Exec(stdinBytes, "mktree", "-z")
	if err != nil {
		if commitVerbose {
			return "", fmt.Errorf("%s\n\nstdin input follows:\n%s", err, stdinBytes)
		}
		return "", err
	}
	return string(bytes.TrimSpace(oidBytes)), nil
}

func hashObject(ctx context.Context, repo gitserver.Repo, typ, path string, data []byte) (oid string, err error) {
	if err := checkSpecArgSafety(typ); err != nil {
		return "", err
	}
	if err := checkSpecArgSafety(path); err != nil {
		return "", err
	}
	cmd := gitserver.DefaultClient.Command("git", "hash-object", "-t", typ, "-w", "--stdin", "--path", path)
	cmd.Repo = repo
	cmd.Stdin
	oidBytes, err := cmd.Output(ctx)
	return string(bytes.TrimSpace(oidBytes)), err
}
