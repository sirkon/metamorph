package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/sirkon/errors"
	"github.com/sirkon/metamorph/internal/app"
	"github.com/sirkon/message"
	"github.com/willabides/kongplete"
)

func main() {
	var cli cliArgs
	cli.Generate.Primary.needLocal = true
	parser := kong.Must(
		&cli,
		kong.Name(app.Name),
		kong.Description(
			`Code generator for conversions between Go structures. Generated code is to be placed where the primary structure is defined`,
		),
		kong.ConfigureHelp(kong.HelpOptions{
			Summary: true,
			Compact: true,
		}),
		kong.UsageOnError(),
	)

	kongplete.Complete(
		parser,
		kongplete.WithPredictor("local-struct-path", &cli.Generate.Primary),
		kongplete.WithPredictor("free-struct-path", &cli.Generate.Secondary),
		kongplete.WithPredictor("outer-package", &packagePath{}),
	)

	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	if err := ctx.Run(&RunContext{
		args: &cli,
	}); err != nil {
		message.Fatal(errors.Wrap(err, "run command"))
	}
}
