# "Metamorph" structs conversions generator

The utility generates conversion functions between primary and secondary structures based on fields names and type 
matching with a dedicated support of types generated with `protoc-gen-go`.

The name comes from Greek **μεταμόρφωση** — metamórfosi — which is (surprise-surprise!) a Greek word for "transformation".


## Installation

```shell
go install github.com/sirkon/metamorph@latest
```


## Usage

* You must be within a module containing a primary package
* Just run
  ```shell
  metamorph generate primary/path:Primary secondary/path:Secondary
  ```
* You may run `metamorph install-completions` to use bash/zsh/whatever completions of packages and structs names.
* There can be no full match. If some fields in either of structs has no match a call for conversion extension (
  or extensions if there's a mismatch for both primary and secondary) will be generated.

Also, remember, if:
* the secondary struct is generated by `protoc-gen-go`
* have a field related to `oneof` of proto
* there are fields with matchable names and types for all branches of this `oneof`

Then the generator will make proper conversions for them too.

## Glossary and definitions

* Primary structure is one that comes first in utility arguments.
* Secondary structure is one that is not primary.
* Primary package is the package containing primary structure.
* Secondary package is the package containing secondary structure.
* Types `A` and `B` can be matched (`A` ≈ `B`) if they meet one of the following criteria:
  * `A` == `B` 
  * `A` is a pointer of `B` or vice versa
  * There is a function or method with no parameters besides `A` itself to convert `A` (or `*A`) into `B` or `*B`
  * `[]A` ≈ `[]B` if `A` ≈ `B`
  * `map[X]A` ≈ `map[Y]B` if `A` ≈ `B` and `X` == `Y`
* Field names `X` and `Y` are matchable if they are both Go-public and `gogh.Underscored(X)` == `gogh.Underscored(Y)`
* Conversion extensions are functions to be called if not all primary or secondary fields were matched. They should be
  defined by user manually.

### TODO

Need to extend from `matchability` to `equivalency` once. That will add transitivity what would mean the generator will 
be able to make a chain of conversions for equivalent types, not just direct ones. I mean, if there are

* types `A`, `B`, `C`
* functions `f: A -> B`, `f': B -> A` and `g: B -> C`, `g': C -> B`

then the generator will know `A` and `C` are matchable via `g∘f: A -> C` and `f'∘g': C -> A`. This will be a true 
relation of equivalency.

Generated functions (or method for primary -> secondary) will be put into the primary package.

## Herustics

metamorph look for conversion functions itself. It can be either method on structure with no arguments in case of
primary -> secondary conversion or a method with a single parameter with one of types and return parameter (and possibly
error) of another type. This may lead to a wrong choice of conversion func. In this case use -x parameter with primary
field name value to exclude it from code generation. The parameter guaranteed there will be a call to not existing 
function where you can write down a proper conversion of the field.

## If heuristcs chose a wrong conversion func/method

metamorph applies heuristics to find conversion functions and it may choose wrong functions at times. You may exclude
fields where it choose wrong conversion funcs. Once a field excluded a manual conversion func call is guaranteed so you may
write a proper conversion.
