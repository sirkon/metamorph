package main

import (
	"os"
	"strings"

	"github.com/sirkon/errors"
	"github.com/sirkon/gogh"
	"github.com/sirkon/jsonexec"
	"github.com/sirkon/metamorph/internal/generator"
	"github.com/sirkon/metamorph/internal/imports"
)

// GenerateCommand generation command
type GenerateCommand struct {
	Primary          structPath  `arg:"" help:"Primary structure to generate conversions in its package. Must look like <rel-path>:<name>." predictor:"local-struct-path"`
	Secondary        structPath  `arg:"" help:"Secondary structure to generate conversions to and from the primary one. Must look like <pkg-path>:<name>." predictor:"free-struct-path"`
	PrimaryMethod    string      `short:"m" help:"MethodPrimary name for the primary -> secondary conversion. Free function will be generated instead if not set."`
	ExcludeFields    []string    `short:"x" help:"Exclude these fields from automatic conversion generation."`
	StructuredErrors packagePath `short:"e" help:"Path to structured errors package." predictor:"outer-package"`
}

// Run запуск генерации
func (c *GenerateCommand) Run(rctx *RunContext) error {
	var listInfo struct {
		Dir  string
		Path string
	}
	if err := jsonexec.Run(&listInfo, "go", "list", "-m", "--json"); err != nil {
		return errors.Wrap(err, "retrieve current module information")
	}

	g, err := generator.New(
		undottedPrefix(c.Primary.pkgPath, listInfo.Path),
		c.Primary.name,
		undottedPrefix(c.Secondary.pkgPath, listInfo.Path),
		c.Secondary.name,
		c.PrimaryMethod,
		c.StructuredErrors.path != "",
		c.ExcludeFields,
	)
	if err != nil {
		return errors.Wrap(err, "setup generator")
	}

	prj, err := gogh.New[*imports.Imports](gogh.FancyFmt, imports.New(c.StructuredErrors.path))
	if err != nil {
		return errors.Wrap(err, "setup matiss for the current project")
	}

	if err := g.Generate(prj); err != nil {
		return errors.Wrap(err, "generate source code")
	}

	if err := prj.Render(); err != nil {
		return errors.Wrap(err, "render generated source code")
	}

	return nil
}

func undottedPrefix(pkg, modPkg string) string {
	if strings.HasPrefix(pkg, "."+string(os.PathSeparator)) {
		return strings.Replace(pkg, "."+string(os.PathSeparator), modPkg+string(os.PathSeparator), 1)
	}

	return pkg
}
