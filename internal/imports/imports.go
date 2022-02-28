package imports

import "github.com/sirkon/gogh"

// New creates a factory of Imports
func New(customErrors string) func(i *gogh.Imports) *Imports {
	return func(i *gogh.Imports) *Imports {
		return &Imports{
			i:          i,
			customErrs: customErrors,
		}
	}
}

// Imports structure to handle imports
type Imports struct {
	i          *gogh.Imports
	customErrs string
}

// Add to satisfy Importer
func (i *Imports) Add(pkgpath string) *gogh.ImportAliasControl {
	return i.i.Add(pkgpath)
}

// Module to satisfy importer
func (i *Imports) Module(relpath string) *gogh.ImportAliasControl {
	return i.i.Module(relpath)
}

// Imports to satisfy importer
func (i *Imports) Imports() *gogh.Imports {
	return i.i
}

// Errors imports custom errors library if set or stdlib errors otherwise
func (i *Imports) Errors() *gogh.ImportAliasControl {
	if i.customErrs != "" {
		return i.i.Add(i.customErrs)
	}

	return i.i.Add("errors")
}

// Fmt imports standard library fmt package
func (i *Imports) Fmt() *gogh.ImportAliasControl {
	return i.i.Add("fmt")
}

var (
	_ gogh.Importer = &Imports{}
)
