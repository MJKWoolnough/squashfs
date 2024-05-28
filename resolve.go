package squashfs

import (
	"io/fs"
	"path"
	"strings"
)

type resolver struct {
	*SquashFS
	fullPath, path     string
	cutAt              int
	redirectsRemaining int
}

func (s *SquashFS) resolve(fpath string, resolveLast bool) (fs.FileInfo, error) {
	if !fs.ValidPath(fpath) {
		return nil, fs.ErrInvalid
	}

	root, err := s.getEntry(s.superblock.RootInode, "")
	if err != nil {
		return nil, err
	}

	const maximumRedirects = 1024

	r := resolver{
		SquashFS:           s,
		fullPath:           fpath,
		path:               fpath,
		redirectsRemaining: maximumRedirects,
	}

	return r.resolve(root, resolveLast)
}

func (r *resolver) resolve(root fs.FileInfo, resolveLast bool) (curr fs.FileInfo, err error) {
	curr = root

	for r.path != "" {
		if curr.Mode()&0o444 == 0 {
			return nil, fs.ErrPermission
		} else if dir, ok := curr.(dirStat); !ok {
			return nil, fs.ErrInvalid
		} else if name := r.splitOffNamePart(); isEmptyName(name) {
			continue
		} else if curr, err = r.getDirEntry(name, dir.blockIndex, dir.blockOffset, dir.fileSize); err != nil {
			return nil, err
		} else if r.isDone(resolveLast) {
			break
		} else if sym, ok := curr.(symlinkStat); !ok {
			continue
		} else if err := r.handleSymlink(sym); err != nil {
			return nil, err
		}

		curr = root
	}

	return curr, nil
}

func (r *resolver) splitOffNamePart() string {
	slashPos := strings.Index(r.path, "/")

	var name string

	if slashPos == -1 {
		name, r.path = r.path, ""
	} else {
		name, r.path = r.path[:slashPos], r.path[slashPos+1:]
		r.cutAt += slashPos + 1
	}

	return name
}

func (r *resolver) handleSymlink(sym symlinkStat) error {
	r.redirectsRemaining--
	if r.redirectsRemaining == 0 {
		return fs.ErrInvalid
	}

	if strings.HasPrefix(sym.targetPath, "/") {
		r.fullPath = path.Clean(sym.targetPath)[1:]
	} else if r.path == "" {
		r.fullPath = path.Join(r.fullPath[:r.cutAt], sym.targetPath)
	} else {
		r.fullPath = path.Join(r.fullPath[:r.cutAt-len(sym.name)-1], sym.targetPath, r.path)
	}

	r.path = r.fullPath
	r.cutAt = 0

	return nil
}

func (r *resolver) isDone(resolveLast bool) bool {
	return r.path == "" && !resolveLast
}

func isEmptyName(name string) bool {
	return name == "" || name == "."
}
