/*
 * Copyright (c) 2021 The GoPlus Authors (goplus.org). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package modload

import (
	"errors"
	"os"
	"path/filepath"

	gomodfile "golang.org/x/mod/modfile"

	"github.com/goplus/gop/env"
	"github.com/goplus/gop/x/mod/modfile"
)

var (
	ErrNoModDecl = errors.New("no module declaration in gop.mod (or go.mod)")
	ErrNoModRoot = errors.New("gop.mod or go.mod file not found in current directory or any parent directory")
)

type Module struct {
	mod *modfile.File
}

func (p Module) Modfile() string {
	return p.mod.Syntax.Name
}

// fixVersion returns a modfile.VersionFixer implemented using the Query function.
//
// It resolves commit hashes and branch names to versions,
// canonicalizes versions that appeared in early vgo drafts,
// and does nothing for versions that already appear to be canonical.
//
// The VersionFixer sets 'fixed' if it ever returns a non-canonical version.
func fixVersion(fixed *bool) modfile.VersionFixer {
	return func(path, vers string) (resolved string, err error) {
		// do nothing
		return vers, nil
	}
}

func Load(dir string) (p Module, err error) {
	gopmod, err := env.GOPMOD(dir)
	if err != nil {
		return
	}

	data, err := os.ReadFile(gopmod)
	if err != nil {
		return
	}

	var fixed bool
	f, err := modfile.Parse(gopmod, data, fixVersion(&fixed))
	if err != nil {
		// Errors returned by modfile.Parse begin with file:line.
		return
	}
	if f.Module == nil {
		// No module declaration. Must add module path.
		return Module{}, ErrNoModDecl
	}
	return Module{mod: f}, nil
}

// -----------------------------------------------------------------------------

func (p Module) UpdateGoMod(checkDirty bool) {
	gopmod := p.Modfile()
	dir, file := filepath.Split(gopmod)
	if file == "go.mod" {
		return
	}
	gomod := dir + "go.mod"
	if checkDirty && isUpdated(gomod, gopmod) {
		return
	}
	gof := p.convToGoMod()
	if b, err := gof.Format(); err == nil {
		os.WriteFile(gomod, b, 0644)
	}
}

func (p Module) convToGoMod() *gomodfile.File {
	copy := p.mod.File
	copy.Syntax = cloneFileSyntax(copy.Syntax)
	addRequireIfNotExist(&copy, "github.com/goplus/gop", env.Version())
	addReplaceIfNotExist(&copy, "github.com/goplus/gop", "", env.GOPROOT(), "")
	return &copy
}

func cloneFileSyntax(syn *gomodfile.FileSyntax) *gomodfile.FileSyntax {
	copySyn := *syn
	stmt := make([]gomodfile.Expr, len(copySyn.Stmt))
	for i, e := range copySyn.Stmt {
		stmt[i] = cloneExpr(e)
	}
	copySyn.Stmt = stmt
	return &copySyn
}

func cloneExpr(e gomodfile.Expr) gomodfile.Expr {
	if v, ok := e.(*gomodfile.LineBlock); ok {
		copy := *v
		return &copy
	}
	return e
}

func addRequireIfNotExist(f *gomodfile.File, path, vers string) {
	for _, r := range f.Require {
		if r.Mod.Path == path {
			return
		}
	}
	f.AddNewRequire(path, vers, false)
}

func addReplaceIfNotExist(f *gomodfile.File, oldPath, oldVers, newPath, newVers string) {
	for _, r := range f.Replace {
		if r.Old.Path == oldPath && (oldVers == "" || r.Old.Version == oldVers) {
			return
		}
	}
	f.AddReplace(oldPath, oldVers, newPath, newVers)
}

func isUpdated(target, src string) bool {
	fiTarget, err := os.Stat(target)
	if err != nil {
		return false
	}
	fiSrc, err := os.Stat(src)
	if err != nil {
		return false
	}
	return fiTarget.ModTime().After(fiSrc.ModTime())
}

// -----------------------------------------------------------------------------
