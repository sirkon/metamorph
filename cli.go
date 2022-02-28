package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/posener/complete"
	"github.com/sirkon/errors"
	"github.com/sirkon/jsonexec"
	"github.com/sirkon/message"
	"github.com/willabides/kongplete"
	"golang.org/x/tools/go/packages"
)

// cliArgs utility arguments
type cliArgs struct {
	Version  VersionCommand  `cmd:"" help:"Print version and exit."`
	Generate GenerateCommand `cmd:"" help:"Generate conversions."`

	InstallCompletions kongplete.InstallCompletions `cmd:"" help:"Install shell completions."`
}

// RunContext command run context
type RunContext struct {
	args *cliArgs
}

type structPath struct {
	needLocal bool
	pkgPath   string
	name      string
}

// UnmarshalText unmarshal for struct arguments
func (p *structPath) UnmarshalText(x []byte) error {
	parts := strings.Split(string(x), ":")
	if len(parts) != 2 {
		return errors.Newf("<pkg-path>:<struct name> value required, got '%s'", string(x))
	}

	p.pkgPath = parts[0]
	p.name = parts[1]

	if p.needLocal {
		// проверка, что задан локальный пакет (относительным путём)
		if !strings.HasPrefix(p.pkgPath, "./") {
			return errors.Newf(
				"pkg-path must be set with relative path against current project — must be in the project root — got '%s'",
				p.pkgPath,
			)
		}
	}

	return nil
}

// Predict to satisfy complete.Predictor
func (p *structPath) Predict(args complete.Args) []string {
	// two choices here: we have already started looking for a structure, or just at package selection yet
	parts := strings.Split(args.Last, ":")
	if len(parts) > 2 {
		return nil
	}

	if len(parts) == 1 {
		var pkgs []string
		if p.needLocal {
			pkgs = p.lookForLocalPackages(parts[0])
		} else {
			switch {
			case parts[0] == "":
				pkgs = p.lookForLocalPackages("")
				pkgs = append(pkgs, p.lookForOuterPackages("")...)
			case strings.HasPrefix(parts[0], "./"):
				pkgs = p.lookForLocalPackages(parts[0])
			default:
				pkgs = p.lookForOuterPackages(parts[0])
			}
		}

		switch len(pkgs) {
		case 0:
			return nil
		case 1:
			// should show structures of a package if only one matched
			return p.lookForPackageStructs(pkgs[0])
		default:
			for i, pkg := range pkgs {
				pkgs[i] = pkg + ":"
			}

			return pkgs
		}
	}

	// chose package, look for structs
	structs := p.lookForPackageStructs(parts[0])
	var res []string
	for _, s := range structs {
		if strings.HasPrefix(s, args.Last) {
			res = append(res, s)
		}
	}

	return res
}

// lookForLocalPackages look for local module packages
func (p *structPath) lookForLocalPackages(prefix string) []string {
	var pkgs []string
	err := filepath.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".idea":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasPrefix(path, "."+string(os.PathSeparator)) {
			path = "." + string(os.PathSeparator) + path
		}

		if !strings.HasPrefix(path, prefix) {
			return nil
		}

		if strings.HasSuffix(path, ".go") {
			dir, _ := filepath.Split(path)
			dir = strings.TrimRight(dir, string(os.PathSeparator))
			if len(pkgs) == 0 {
				pkgs = append(pkgs, dir)
			} else {
				if pkgs[len(pkgs)-1] != dir {
					pkgs = append(pkgs, dir)
				}
			}
		}

		return nil
	})
	if err != nil {
		message.Fatal(errors.Wrap(err, "look for local packages"))
	}

	return pkgs
}

func (p *structPath) lookForGoPkgsInRoot(root string) ([]string, error) {
	var pkgs []string
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".idea":
				return filepath.SkipDir
			default:
				return nil
			}
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		dir, _ := filepath.Split(path)
		dir = strings.TrimRight(dir, string(os.PathSeparator))
		pkg := strings.TrimLeft(dir[len(root):], string(os.PathSeparator))

		pkgs = append(pkgs, pkg)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	return pkgs, nil
}

func (p *structPath) lookForOuterPackages(prefix string) []string {
	var modInfo struct {
		Require []struct {
			Path string
		}
	}
	if err := jsonexec.Run(&modInfo, "go", "mod", "edit", "--json"); err != nil {
		message.Fatal(errors.Wrap(err, "get list of module dependencies"))
	}

	var pkgs []string
	for _, r := range modInfo.Require {
		// this may be the case when we are within a module already
		if strings.HasPrefix(prefix, r.Path) {
			// exactly, look for directories of a package
			var listInfo struct {
				Dir string
			}
			if err := jsonexec.Run(&listInfo, "go", "list", "--json", "-m", r.Path); err != nil {
				message.Fatal(errors.Wrap(err, "[1] look for package directory of an outer package"))
			}

			rel := strings.TrimLeft(prefix[len(r.Path):], string(os.PathSeparator))
			subpkgs, err := p.lookForGoPkgsInRoot(listInfo.Dir)
			if err != nil {
				message.Fatal(errors.Wrap(err, "[1] look for subpackages of "+r.Path))
			}

			for _, subpkg := range subpkgs {
				if strings.HasPrefix(subpkg, rel) {
					pkgs = append(pkgs, filepath.Join(r.Path, subpkg))
				}
			}

			continue
		}

		if !strings.HasPrefix(r.Path, prefix) {
			continue
		}

		// there can be packages of the module, look for them
		var listInfo struct {
			Dir string
		}
		if err := jsonexec.Run(&listInfo, "go", "list", "--json", "-m", r.Path); err != nil {
			message.Fatal(errors.Wrap(err, "[1] look for package directory of an outer package"))
		}

		subpkgs, err := p.lookForGoPkgsInRoot(listInfo.Dir)
		if err != nil {
			message.Fatal(errors.Wrap(err, "[2] look for subpackages of "+r.Path))
		}

		for _, subpkg := range subpkgs {
			pkgs = append(pkgs, filepath.Join(r.Path, subpkg))
		}
	}

	sort.Strings(pkgs)
	return pkgs
}

func (p *structPath) lookForPackageStructs(pkg string) []string {
	var listInfo struct {
		Dir string
	}
	if err := jsonexec.Run(&listInfo, "go", "list", "--json", pkg); err != nil {
		message.Fatal(errors.Wrap(err, "look for package directory of "+pkg))
	}

	dirItems, err := os.ReadDir(listInfo.Dir)
	if err != nil {
		message.Fatal(errors.Wrap(err, "look for directory items"))
	}

	var structs []string
	for _, item := range dirItems {
		if item.IsDir() {
			continue
		}

		if !strings.HasSuffix(item.Name(), ".go") {
			continue
		}

		res, err := p.lookForFileStructs(listInfo.Dir, item.Name())
		if err != nil {
			message.Fatal(errors.Wrapf(err, "look for structs in %s", filepath.Join(listInfo.Dir, item.Name())))
		}

		for _, name := range res {
			structs = append(structs, pkg+":"+name)
		}
	}

	return structs
}

func (p *structPath) lookForFileStructs(dir, name string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.AllErrors)
	if err != nil {
		return nil, errors.Wrap(err, "parse file")
	}

	var structs []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch ts := node.(type) {
		case *ast.TypeSpec:
			if _, ok := ts.Type.(*ast.StructType); !ok {
				return true
			}

			structs = append(structs, ts.Name.Name)
			return false
		}

		return true
	})

	return structs, nil
}

type packagePath struct {
	path string
}

// UnmarshalText to check presumably structured errors package. The package should follow these conditions:
//   * Must have functions
//       New(string) E
//       Newf(string, ...interface{}) E
//       Wrap(error, string) E
//       Wrapf(error, string, ...interface{}) E
//   * Type E must be defined in this package as well
//   * Type E must implements error
//   * Type E must have Any(string, interface{}) method
func (p *packagePath) UnmarshalText(text []byte) error {
	pkgpath := string(text)
	fset := token.NewFileSet()
	pkgs, err := packages.Load(
		&packages.Config{
			Mode: packages.NeedImports | packages.NeedTypes | packages.NeedName | packages.NeedDeps |
				packages.NeedSyntax | packages.NeedFiles | packages.NeedModule,
			Fset:  fset,
			Tests: false,
		},
		pkgpath,
	)
	if err != nil {
		return errors.Wrap(err, "parse "+pkgpath)
	}

	if len(pkgs) == 0 {
		return errors.Newf("package %s was not found", pkgpath)
	}

	pkg := pkgs[0]
	foundFuncs := map[string]*types.Func{
		"New":   nil,
		"Newf":  nil,
		"Wrap":  nil,
		"Wrapf": nil,
	}

	var errType types.Type
	for fname := range foundFuncs {
		fun := pkg.Types.Scope().Lookup(fname)
		if fun == nil {
			return errors.Newf("function %s is missing in %s", fname, pkgpath)
		}

		typ := fun.Type().(*types.Signature)
		if typ.Recv() != nil {
			return errors.Newf("function %s.%s must not be a method", pkgpath, fname)
		}

		if typ.Results().Len() != 1 {
			return errors.Newf("function %s.%s returns %d values", pkgpath, fname, typ.Results().Len())
		}

		rtyp := typ.Results().At(0)
		if errType != nil {
			if !types.AssignableTo(errType, rtyp.Type()) {
				return errors.Newf("unexpected result type %s of function %s.%s", rtyp.Type(), pkgpath, fname)
			}

			continue
		}

		var tmptyp types.Type
		switch v := rtyp.Type().(type) {
		case *types.Pointer:
			tmptyp = v.Elem()
		default:
			tmptyp = rtyp.Type()
		}

		switch v := tmptyp.(type) {
		case *types.Named:
			// must be in the same package
			if v.Obj().Pkg().Path() != pkgpath {
				return errors.Newf("return type of %s.%s must be in the same package", pkgpath, fname)
			}

			var implementsError bool
			for i := 0; i < v.NumMethods(); i++ {
				method := v.Method(i)
				if method.Name() == "Error" {
					et := method.Type().(*types.Signature)
					if et.Results().Len() != 1 ||
						et.Params().Len() != 0 ||
						!isStringType(et.Results().At(0).Type()) {
						return errors.Newf("type %s.%s does not implement error", pkgpath, fname)
					}

					implementsError = true
				}

				if method.Name() != "Any" {
					continue
				}

				tm := method.Type().(*types.Signature)
				if tm.Results().Len() != 1 {
					return errors.Newf("method %s.%s.Any must have the single return value", pkgpath, fname)
				}

				if !types.AssignableTo(tm.Results().At(0).Type(), rtyp.Type()) {
					return errors.Newf("method %s.%s.Any must return its receiver's type", pkgpath, fname)
				}

				if tm.Params().Len() != 2 {
					return errors.Newf("method %s.%s.Any must have two params", pkgpath, fname)
				}

				if !isStringType(tm.Params().At(0).Type()) {
					return errors.Newf("method %s.%s.Any first parameter must have string type", pkgpath, fname)
				}

				switch tm.Params().At(1).Type().String() {
				case "interface{}", "any":
				default:
					return errors.Newf(
						"method %s.%s.Any second parameter must be interface{} or any type",
						pkgpath,
						fname,
					)
				}
			}

			if !implementsError {
				return errors.Newf("type %s.%s does not implement error", pkgpath, fname)
			}
		default:
			return errors.Newf("invalid result type of function %s.%s", pkgpath, fname)
		}

		errType = rtyp.Type()
	}

	p.path = pkgpath
	return nil
}

// Predict to implement complete.Predictor
func (p *packagePath) Predict(args complete.Args) []string {
	var modInfo struct {
		Require []struct {
			Path string
		}
	}
	if err := jsonexec.Run(&modInfo, "go", "mod", "edit", "--json"); err != nil {
		message.Fatal(errors.Wrap(err, "get list of module dependencies"))
	}

	var pkgs []string
	for _, r := range modInfo.Require {
		if strings.HasPrefix(args.Last, r.Path) {
			var listInfo struct {
				Dir string
			}
			if err := jsonexec.Run(&listInfo, "go", "list", "--json", "-m", r.Path); err != nil {
				message.Fatal(errors.Wrap(err, "[1] look for package directory of an outer package"))
			}

			rel := strings.TrimLeft(args.Last[len(r.Path):], string(os.PathSeparator))
			subpkgs, err := p.lookForGoPkgsInRoot(listInfo.Dir)
			if err != nil {
				message.Fatal(errors.Wrap(err, "[1] look for subpackages of "+r.Path))
			}

			for _, subpkg := range subpkgs {
				if strings.HasPrefix(subpkg, rel) {
					pkgs = append(pkgs, filepath.Join(r.Path, subpkg))
				}
			}

			continue
		}

		if !strings.HasPrefix(r.Path, args.Last) {
			continue
		}

		var listInfo struct {
			Dir string
		}
		if err := jsonexec.Run(&listInfo, "go", "list", "--json", "-m", r.Path); err != nil {
			message.Fatal(errors.Wrap(err, "[1] look for package directory of an outer package"))
		}

		subpkgs, err := p.lookForGoPkgsInRoot(listInfo.Dir)
		if err != nil {
			message.Fatal(errors.Wrap(err, "[2] look for subpackages of "+r.Path))
		}

		for _, subpkg := range subpkgs {
			pkgs = append(pkgs, filepath.Join(r.Path, subpkg))
		}
	}

	sort.Strings(pkgs)
	return pkgs
}

func (p *packagePath) lookForGoPkgsInRoot(root string) ([]string, error) {
	var pkgs []string
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".idea":
				return filepath.SkipDir
			default:
				return nil
			}
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		dir, _ := filepath.Split(path)
		dir = strings.TrimRight(dir, string(os.PathSeparator))
		pkg := strings.TrimLeft(dir[len(root):], string(os.PathSeparator))

		pkgs = append(pkgs, pkg)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	return pkgs, nil
}

func isStringType(t types.Type) bool {
	v, ok := t.(*types.Basic)
	if !ok {
		return false
	}

	return v.Kind() == types.String
}
