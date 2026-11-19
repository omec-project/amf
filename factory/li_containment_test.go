// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestStrictLiBlockDoesNotFailTheConfigurationLoad is a source-level assertion.
//
// `service/li_block_containment_test.go` asserts that the Lawful Interception block cannot return
// from Start. Its guards cannot see this package: a refusal returned from InitConfigFactory stops
// this network function over a single mistyped LI key one call frame earlier, and they still pass.
// A guard scoped to one call site reads as coverage while the property is violated at another.
//
// So the rule here is the other half: `strictLiBlock`'s verdict is *recorded*, never returned.
// Returning it fails InitConfigFactory, which fails Initialize, which reaches a Fatalf — taking
// the service-based interface, NGAP and every UE this AMF serves with it, over an optional
// subsystem. It is also the loudest possible disclosure that this element is LI-provisioned: a
// network function that will not start is visible to every operator, every peer and every
// monitoring system, where a log line is visible only to whoever reads logs.
//
// The refusal still happens, and interception still does not start on it. The LI subsystem acts
// on it, at a point where the ADMF can be told. See LiBlockError.
func TestStrictLiBlockDoesNotFailTheConfigurationLoad(t *testing.T) {
	const file = "factory.go"

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	var found, recorded bool

	ast.Inspect(parsed, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == "strictLiBlock" {
				found = true
			}
		}

		return true
	})

	if !found {
		t.Fatalf("no call to strictLiBlock in %s; if the strict LI decode moved, move this guard "+
			"with it rather than deleting it — the property it holds is that a refused LI block "+
			"never stops the network function", file)
	}

	// The call must be the right-hand side of an assignment to liBlockErr, and nothing else. An
	// `if err := strictLiBlock(...); err != nil { return err }` is the shape that fails the load,
	// and it is an assignment too — so the guard checks the target, not merely that an
	// assignment happened.
	ast.Inspect(parsed, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Rhs) != 1 {
			return true
		}

		call, isCall := assign.Rhs[0].(*ast.CallExpr)
		if !isCall {
			return true
		}

		if ident, isIdent := call.Fun.(*ast.Ident); !isIdent || ident.Name != "strictLiBlock" {
			return true
		}

		for _, lhs := range assign.Lhs {
			if target, ok := lhs.(*ast.Ident); ok && target.Name == "liBlockErr" {
				recorded = true
			}
		}

		return true
	})

	if !recorded {
		t.Errorf("%s: strictLiBlock's verdict is not recorded in liBlockErr. If it is returned "+
			"from InitConfigFactory instead, a single mistyped key in the optional `li` block "+
			"stops this whole network function — an outage, and the loudest way to disclose that "+
			"this element is LI-provisioned. Record it and let the LI subsystem refuse "+
			"interception on it, where the ADMF can be told.", file)
	}
}

// TestTheLenientDecodeDoesNotReadTheLiBlock is the half of the containment rule that lives in
// config.go. Recording strictLiBlock's verdict contains only what strictLiBlock sees. If the
// lenient decode of the whole file also decodes the block, a value of the wrong type there — a
// `deactivateAllTasks: perhaps` — is that decode's error, InitConfigFactory returns it, and the
// AMF stops. So Config's Li field must carry `yaml:"-"`, leaving strictLiBlock the block's only
// decoder.
func TestTheLenientDecodeDoesNotReadTheLiBlock(t *testing.T) {
	const file = "config.go"

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	var tag string

	found := false

	ast.Inspect(parsed, func(n ast.Node) bool {
		spec, isSpec := n.(*ast.TypeSpec)
		if !isSpec || spec.Name.Name != "Configuration" {
			return true
		}

		st, isStruct := spec.Type.(*ast.StructType)
		if !isStruct {
			return false
		}

		for _, field := range st.Fields.List {
			for _, name := range field.Names {
				if name.Name != "Li" {
					continue
				}

				found = true

				if field.Tag != nil {
					unquoted, err := strconv.Unquote(field.Tag.Value)
					if err != nil {
						t.Fatalf("%s: Li's tag %s does not unquote: %v",
							fset.Position(field.Pos()), field.Tag.Value, err)
					}

					tag = reflect.StructTag(unquoted).Get("yaml")
				}
			}
		}

		return false
	})

	if !found {
		t.Fatalf("no Li field in Configuration in %s; if the LI block moved, move this guard with "+
			"it rather than deleting it", file)
	}

	if tag != "-" {
		t.Errorf("%s: Configuration.Li is tagged yaml:%q, not yaml:\"-\". The lenient decode of "+
			"the whole file then reads the li block too, and a value of the wrong type there fails "+
			"the configuration load, which stops this network function over an optional "+
			"subsystem. Leave the block to strictLiBlock.", file, tag)
	}
}

// TestAMisspelledLiKeyDoesNotStopTheAMF asserts both halves: the key must be refused, *and* the
// network function must still come up.
func TestAMisspelledLiKeyDoesNotStopTheAMF(t *testing.T) {
	const good = `info:
  version: 1.0.0
configuration:
  amfName: AMF
  li:
    x1Listen: ":8443"
    neId: amf-1
    admfUrl: https://admf:9443
    keepaliveTimeout: 30s
`

	write := func(t *testing.T, body string) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), "amfcfg.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		return path
	}

	t.Run("a conformant li block leaves no refusal recorded", func(t *testing.T) {
		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig; liBlockErr = nil })

		if err := InitConfigFactory(write(t, good)); err != nil {
			t.Fatalf("a conformant li block was refused: %v", err)
		}
		// A reload that fixes the block must clear the recorded refusal, or one bad load
		// disables interception for the life of the process.
		if err := LiBlockError(); err != nil {
			t.Errorf("a conformant li block left a refusal recorded: %v", err)
		}
	})

	for _, typo := range []string{typoKeepaliveTimeout, typoAdmfURL} {
		t.Run(typo, func(t *testing.T) {
			orig := AmfConfig
			t.Cleanup(func() { AmfConfig = orig; liBlockErr = nil })

			body := strings.Replace(good, "keepaliveTimeout", typo, 1)
			if typo == typoAdmfURL {
				body = strings.Replace(good, "admfUrl", typo, 1)
			}

			if err := InitConfigFactory(write(t, body)); err != nil {
				t.Fatalf("a misspelled LI key stopped the whole configuration load, which stops "+
					"the network function: %v", err)
			}

			err := LiBlockError()
			if err == nil {
				t.Fatalf("%s was accepted, so the setting the operator wrote never reached the "+
					"element and its unsafe default stands with nothing saying so", typo)
			}
			if !strings.Contains(err.Error(), typo) {
				t.Errorf("the refusal does not name the key that was wrong: %v", err)
			}
		})
	}
}

// TestTheStrictLiDecodeIsStillStrict guards the other direction. Recording rather than returning
// the refusal changes only who acts on it, and must not be read, or implemented, as backing out
// the strictness that produces it.
func TestTheStrictLiDecodeIsStillStrict(t *testing.T) {
	const body = `info:
  version: 1.0.0
configuration:
  li:
    x1Listen: ":8443"
    neId: amf-1
    keepaliveTimeut: 30s
`

	if _, err := strictLiBlock([]byte(body)); err == nil {
		t.Error("a misspelled LI key was accepted by the strict decode; the containment fix is " +
			"about who acts on the refusal, not about whether there is one")
	}
}

// TestAnLiTypeErrorDoesNotStopTheAMF asserts that a value of the wrong type in the li block is
// handled like a misspelled key: recorded for the LI subsystem, with the configuration load
// succeeding and the block still handed on, so that the LI subsystem starts far enough to tell
// the ADMF it is not intercepting. A type error outside the li block still fails the load: the
// containment is scoped to LI.
func TestAnLiTypeErrorDoesNotStopTheAMF(t *testing.T) {
	base, err := os.ReadFile("../util/testdata/amfcfg.yaml")
	if err != nil {
		t.Fatalf("read base config: %v", err)
	}

	if !strings.Contains(string(base), "\nconfiguration:\n") {
		t.Fatal("the base configuration has no top-level configuration key to add to")
	}

	load := func(t *testing.T, add string) error {
		t.Helper()

		orig := AmfConfig
		t.Cleanup(func() { AmfConfig = orig; liBlockErr = nil })

		body := strings.Replace(string(base), "\nconfiguration:\n", "\nconfiguration:\n"+add, 1)
		path := filepath.Join(t.TempDir(), "amfcfg.yaml")

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		return InitConfigFactory(path)
	}

	for _, block := range []string{
		"  li: {neId: amf-1, deactivateAllTasks: perhaps}\n",
		"  li: {neId: amf-1, mdf2: [a, b]}\n",
		"  li: perhaps\n",
	} {
		t.Run(strings.TrimSpace(block), func(t *testing.T) {
			if err := load(t, block); err != nil {
				t.Fatalf("a value of the wrong type in the li block failed the whole configuration "+
					"load, which stops the AMF: %v", err)
			}

			if LiBlockError() == nil {
				t.Error("a value of the wrong type in the li block was not refused")
			}

			// service/init.go starts the LI subsystem only on a non-nil block, and it is the LI
			// subsystem that reports the refusal to the ADMF. A nil here turns the refusal into
			// silence.
			if AmfConfig.Configuration.Li == nil {
				t.Error("a refused li block was dropped, so the refusal is never reported")
			}
		})
	}

	for _, block := range []string{"  li:\n", "  li: ~\n", ""} {
		t.Run("off: "+strings.TrimSpace(block), func(t *testing.T) {
			if err := load(t, block); err != nil {
				t.Fatalf("load: %v", err)
			}

			if err := LiBlockError(); err != nil {
				t.Errorf("an absent or null li block was refused: %v", err)
			}

			if AmfConfig.Configuration.Li != nil {
				t.Errorf("an absent or null li block turned interception on: %+v",
					AmfConfig.Configuration.Li)
			}
		})
	}

	t.Run("a type error outside the li block still fails the load", func(t *testing.T) {
		if err := load(t, "  ngappPort: abc\n"); err == nil {
			t.Error("a value of the wrong type outside the li block was accepted; the " +
				"containment must stay scoped to LI")
		}
	})
}
