package repl

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/manishmeganathan/symbolizer"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/cmd/logiclab/core"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
)

func HelpManifest() string {
	return `
@>Manifests<@ are composite deployment artifacts for [@>logics<@] and are usually found as JSON, YAML or POLO files.
Logic Manifest Spec: https://sarvalabs.notion.site/Logic-Manifest-Standard-93f5fee1af8d4c3cad155b9827b97930?pvs=4 

LogicLab can manipulate Manifests to [@>deploy<@] as a logic or [@>convert<@] to other formats using the manifest
expression which accepts either a relative filepath (string) or a raw manifest bytes (hex-encoded polo data). 
Using the expression directly will print the manifest back in its hex-encoded polo form.

supported expression formats:
@>manifest("[filepath]")<@
@>manifest([hex-encoded polo])<@
`
}

func parseManifestCommand(parser *symbolizer.Parser) Command {
	manifest, _, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	return func(repl *Repl) string {
		return core.PrintManifest(manifest, common.POLO)
	}
}

// returns a manifest, its filepath (empty for raw manifest expressions) and an error
func parseManifestExpression(parser *symbolizer.Parser) (engineio.Manifest, string, error) {
	if !parser.ExpectPeek(symbolizer.TokenKind('(')) {
		return nil, "", errors.New("invalid manifest expression: missing '('")
	}

	// Parse the expression contents inside the ()
	inner, err := parser.Unwrap(symbolizer.EnclosureParens())
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid manifest expression")
	}

	parserInner := symbolizer.NewParser(inner)
	// Check the inner value is valid
	if !parserInner.Cursor().Kind.CanValue() {
		return nil, "", errors.New("invalid manifest expression")
	}

	// Parse the value within the parenthesis
	value, err := parserInner.Cursor().Value()
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid manifest expression")
	}

	switch parserInner.Cursor().Kind {
	case symbolizer.TokenString:
		// value is a string
		fpath, _ := value.(string)

		// Read the manifest at the given filepath
		manifest, err := engineio.NewManifestFromFile(fpath)
		if err != nil {
			return nil, "", fmt.Errorf("invalid manifest file: %w", err)
		}

		return manifest, fpath, nil

	case symbolizer.TokenHexNumber:
		// value is some bytes
		raw, _ := value.([]byte)

		manifest, err := engineio.NewManifest(raw, common.POLO)
		if err != nil {
			return nil, "", errors.Wrap(err, "invalid raw manifest")
		}

		return manifest, "", nil

	default:
		return nil, "", errors.New("invalid manifest expression")
	}
}

func HelpConvert() string {
	return `
The @>convert<@ can be used to convert a [@>manifest<@] into other encoding formats 
(with the @>as<@ preposition) or code formats (with the @>into<@ preposition). 

Supported Encoding Values: @>JSON<@, @>YAML<@ and @>POLO<@
Supported Codeform Values: @>BIN<@, @>HEX<@ and @>ASM<@

usage:
@>convert manifest(...) as [encoding]<@
@>convert manifest(...) into [codeform]<@

examples:
>> convert manifest("./manifests/ledger.yaml") as JSON
// prints JSON object

>> convert manifest("./manifests/ledger.yaml") as POLO
// prints hex encoded string of POLO bytes

>> convert manifest("./manifests/ledger.yaml") into HEX
// prints manifest with HEX code
`
}

func parseConvertCommand(parser *symbolizer.Parser) Command {
	if !parser.ExpectPeek(TokenManifest) {
		return InvalidCommandError("missing manifest expression for convert")
	}

	manifest, fpath, err := parseManifestExpression(parser)
	if err != nil {
		return InvalidCommandError(err.Error())
	}

	switch parser.Cursor().Kind {
	// Convert Encoding Format [JSON, YAML, POLO]
	case TokenPrepositionAs:
		if !parser.ExpectPeek(TokenManifestEncoding) {
			return InvalidCommandError("missing encoding format for convert")
		}

		return func(repl *Repl) string {
			return core.PrintManifest(manifest, common.EncodingFromString(parser.Cursor().Literal))
		}

	// Convert Codeform [BIN, HEX, ASM]
	case TokenPrepositionInto:
		if !parser.ExpectPeek(TokenManifestCodeform) {
			return InvalidCommandError("missing codeform for convert")
		}

		extension := strings.TrimPrefix(filepath.Ext(fpath), ".")
		encoding := strings.ToUpper(extension)

		return func(repl *Repl) string {
			return core.ConvertManifestCodeform(manifest, common.EncodingFromString(encoding), parser.Cursor().Literal)
		}

	default:
		return InvalidCommandErrorf("invalid preposition after manifest expr for convert")
	}
}
