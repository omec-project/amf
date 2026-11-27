// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

// liBlockErr records why the `li` block was refused, if it was. Package-level because
// InitConfigFactory has no other channel to the LI subsystem, and it is reset on every load so a
// reload that fixes the block clears it.
var liBlockErr error

// LiBlockError reports why the `li` block was refused, or nil. The LI subsystem consults it at
// start-up: interception does not start on a refusal, and the refusal is reported to the ADMF.
//
// **The caller must not log this, and must not fail the network function on it.** The text names
// LI configuration keys, so it belongs on the X1 fault channel and nowhere else.
func LiBlockError() error { return liBlockErr }

// strictLiBlock decodes the `li` block on its own, refusing a key this element does not model and
// a value of the wrong type. It returns nil when the document has no `li` key, or a null one:
// interception is off. Otherwise it returns the block as far as it decoded, with the refusal, if
// any, beside it.
//
// **This is the only decode of the block.** Config tags `Li` with `yaml:"-"`, so the lenient
// decode of the whole file never sees it. A value of the wrong type in the block — a
// `deactivateAllTasks: perhaps` — is then this decode's refusal, scoped to interception, rather
// than an error from the whole configuration load, which stops the AMF.
//
// **A refused block is still returned, never nil.** The decode fills what it can read and
// reports the rest, so when admfUrl is among the fields that did decode, the LI subsystem can
// report the refusal to the ADMF. A block that is not a mapping at all decodes nothing and is
// refused all the same: returning nil for it would read as "interception is off" and the refusal
// would never be acted on.
//
// **A misspelled LI key is silently dropped, and the defaults it lands on are the dangerous
// ones.** A typo'd `keepaliveRetainTasking` or `keepaliveTimeout` leaves the X1 keepalive
// procedure on timers or a purge policy the operator did not choose, while the element looks
// healthy. A typo'd `admfUrl` leaves the fault channel a no-op, so every condition this element
// is required to report goes nowhere — including the report that would have said the
// configuration was wrong. In both cases the operator wrote the setting, the element ignored it,
// and nothing anywhere says so.
//
// **Scoped to the `li` block, deliberately.** Decoding the whole configuration strictly would
// refuse every key this version does not model, so a configuration carrying a field added
// later would stop the AMF starting. The property to hold is narrow — a mistyped LI key must not
// reach a default — and so is the check.
//
// The block is isolated first and decoded strictly second, because strictness lives on the decode
// and a decode covers everything it is given. The lenient first pass models nothing but the path
// to the block, so no key outside it is visible to be refused.
func strictLiBlock(content []byte) (*Li, error) {
	// Only the path to the block. This struct must not grow: modelling anything else here would
	// make it a second, diverging definition of the configuration.
	var outer struct {
		Configuration struct {
			Li yaml.Node `yaml:"li"`
		} `yaml:"configuration"`
	}
	if err := yaml.Unmarshal(content, &outer); err != nil {
		// A repeated `li` key is this check's to refuse; see withoutRepeatedLi. The first block
		// is read as far as it reads, so the refusal can reach the ADMF it names.
		if _, config, keys := configurationLi(content); len(keys) > 1 {
			li := &Li{}
			if raw, err := yaml.Marshal(config.Content[keys[0]+1]); err == nil {
				//nolint:errcheck // the refusal below is the verdict; this only fills what reads
				_ = yaml.Load(raw, li, yaml.WithKnownFields())
			}
			return li, fmt.Errorf("configuration.li: the key appears %d times, and which of the "+
				"blocks was meant cannot be told", len(keys))
		}
		// Anything else that makes the file undecodable is the real decode's to report, with its
		// own message. This check has nothing to add about it.
		return nil, nil
	}

	block := &outer.Configuration.Li
	if block.Kind == 0 || block.ShortTag() == "!!null" {
		// No `li` block, or an empty one: interception is off, and there is nothing to be strict
		// about.
		return nil, nil
	}

	li := &Li{}

	// Round-tripped so the decode sees the block and only the block. One small mapping, once,
	// at startup.
	raw, err := yaml.Marshal(block)
	if err != nil {
		return li, fmt.Errorf("configuration.li: %w", err)
	}

	if err := yaml.Load(raw, li, yaml.WithKnownFields()); err != nil {
		return li, fmt.Errorf("configuration.li: %w — a key this element does not recognise, or a "+
			"value it cannot read, is a setting that never reaches it, and the defaults it leaves in "+
			"place are the unsafe ones: an unread keepalive setting leaves the X1 keepalive procedure "+
			"on terms the operator did not choose, and an unread admfUrl leaves "+
			"this element with no channel to report that on", err)
	}

	return li, nil
}

// configurationLi parses content into a node tree and finds every `li` key of its configuration
// mapping: doc is the tree, config the mapping, and keys the indices of the `li` keys in its
// Content, each followed by its value. keys is empty when there is no such mapping.
func configurationLi(content []byte) (doc, config *yaml.Node, keys []int) {
	doc = &yaml.Node{}
	// A node tree holds a repeated key as written, where a decode into Go values refuses it.
	if err := yaml.Unmarshal(content, doc); err != nil || len(doc.Content) == 0 {
		return nil, nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "configuration" && root.Content[i+1].Kind == yaml.MappingNode {
			config = root.Content[i+1]
		}
	}
	if config == nil {
		return nil, nil, nil
	}
	for i := 0; i+1 < len(config.Content); i += 2 {
		if config.Content[i].Value == "li" {
			keys = append(keys, i)
		}
	}
	return doc, config, keys
}

// withoutRepeatedLi returns content without its `li` blocks when configuration names `li` more
// than once, and false otherwise.
//
// A mapping that repeats a key is not valid YAML, so the decode of the whole configuration
// refuses it, and a refused configuration load stops the AMF. With the blocks taken out, the rest
// of the configuration decodes as written, and the repeated key is refused by strictLiBlock as
// any unusable `li` block is: interception does not start, and the LI subsystem says why to the
// ADMF only.
func withoutRepeatedLi(content []byte) ([]byte, bool) {
	doc, config, keys := configurationLi(content)
	if len(keys) < 2 {
		return nil, false
	}
	kept := make([]*yaml.Node, 0, len(config.Content))
	for i := 0; i+1 < len(config.Content); i += 2 {
		if config.Content[i].Value != "li" {
			kept = append(kept, config.Content[i], config.Content[i+1])
		}
	}
	config.Content = kept
	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, false
	}
	return out, true
}
