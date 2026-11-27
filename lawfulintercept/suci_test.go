// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"bytes"
	"testing"

	"github.com/omec-project/util/li/iri"
)

// suciNAS builds the raw 5GS mobile identity octets a UE sends for a SUCI, so the
// cases below are written in the encoding the AMF actually receives rather than in
// the parser's own terms. MCC 262 / MNC 01 throughout.
//
//	[0] SUPI format 0 (IMSI), identity type 1 (SUCI)
//	[1] MCC digit 2 | digit 1
//	[2] MNC digit 3 | MCC digit 3
//	[3] MNC digit 2 | digit 1
func suciNAS(routing [2]byte, scheme, hnpki byte, output ...byte) []byte {
	return append([]byte{
		0x01,
		0x62, // MCC digits 2,6 -> "26"
		0xf2, // MNC digit 3 = f (two-digit MNC), MCC digit 3 = 2 -> MCC "262"
		0x10, // MNC digits 0,1 -> "01"
		routing[0], routing[1],
		scheme, hnpki,
	}, output...)
}

// The assertions are per member. A parse that is wrong rather than absent produces a
// well-formed record naming a subscriber the network never saw, so "a SUCI came back"
// is not what needs checking.
func TestSuciFromNASReadsEveryMember(t *testing.T) {
	got, ok := suciFromNAS(suciNAS([2]byte{0x21, 0xf3}, 0x01, 0x1B, 0xDE, 0xAD, 0xBE, 0xEF))
	if !ok {
		t.Fatal("a well-formed SUCI was rejected")
	}
	if got.MCC != "262" {
		t.Errorf("MCC = %q, want 262", got.MCC)
	}
	if got.MNC != "01" {
		t.Errorf("MNC = %q, want 01", got.MNC)
	}
	// routing octets 0x21 0xf3 -> nibble-swapped "12" "3f" -> digits "123"
	if got.RoutingIndicator != 123 {
		t.Errorf("RoutingIndicator = %d, want 123", got.RoutingIndicator)
	}
	if got.RoutingIndicatorLength != nil {
		t.Errorf("RoutingIndicatorLength = %d, want absent: 123 has three meaningful digits "+
			"and the integer renders as three, so the module does not ask for it",
			*got.RoutingIndicatorLength)
	}
	if got.ProtectionSchemeID != 1 {
		t.Errorf("ProtectionSchemeID = %d, want 1", got.ProtectionSchemeID)
	}
	if !bytes.Equal(got.HomeNetworkPublicKeyID, iri.HomeNetworkPublicKeyID{0x1B}) {
		t.Errorf("HomeNetworkPublicKeyID = %x, want 1b — the string form renders this with %%d, "+
			"so a value above 9 is where a string-parsing implementation diverges",
			got.HomeNetworkPublicKeyID)
	}
	if !bytes.Equal(got.SchemeOutput, iri.SchemeOutput{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("SchemeOutput = %x, want deadbeef", got.SchemeOutput)
	}
}

// The leading-zero case is the whole reason routingIndicatorLength exists, and it is
// invisible on the wire: a record carrying 123 with no length is well formed and names
// a different routing indicator than the UE sent.
func TestSuciRoutingIndicatorLengthIsCarriedWhenItDiffers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		octets     [2]byte
		wantValue  iri.RoutingIndicator
		wantLength int // 0 means the member must be absent
	}{
		{"four digits, no leading zero", [2]byte{0x21, 0x43}, 1234, 0},
		{"four digits with a leading zero", [2]byte{0x10, 0x32}, 123, 4},
		{"three digits", [2]byte{0x21, 0xf3}, 123, 0},
		{"two digits with a leading zero", [2]byte{0x10, 0xff}, 1, 2},
		{"one digit", [2]byte{0xf0, 0xff}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := suciFromNAS(suciNAS(tc.octets, 0x00, 0x00, 0x01))
			if !ok {
				t.Fatal("rejected a well-formed SUCI")
			}
			if got.RoutingIndicator != tc.wantValue {
				t.Errorf("RoutingIndicator = %d, want %d", got.RoutingIndicator, tc.wantValue)
			}
			switch {
			case tc.wantLength == 0 && got.RoutingIndicatorLength != nil:
				t.Errorf("RoutingIndicatorLength = %d, want absent", *got.RoutingIndicatorLength)
			case tc.wantLength != 0 && got.RoutingIndicatorLength == nil:
				t.Errorf("RoutingIndicatorLength absent, want %d — without it the mediation "+
					"function cannot recover the identifier the UE sent", tc.wantLength)
			case tc.wantLength != 0 && int(*got.RoutingIndicatorLength) != tc.wantLength:
				t.Errorf("RoutingIndicatorLength = %d, want %d", *got.RoutingIndicatorLength, tc.wantLength)
			}
		})
	}
}

// Every rejection below would otherwise become a target identity that is merely
// plausible. Absent is the only safe answer.
func TestSuciFromNASRefusesWhatItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		buf  []byte
	}{
		{"empty", nil},
		{"shorter than the fixed part", []byte{0x01, 0x62, 0xf2, 0x10, 0x21, 0xf3, 0x00, 0x00}},
		{"NAI format, which SUCI ::= SEQUENCE cannot express", append([]byte{0x11}, make([]byte, 12)...)},
		{"not a SUCI at all", append([]byte{0x02}, make([]byte, 12)...)},
		{"routing indicator with a digit after the padding", suciNAS([2]byte{0xf1, 0x32}, 0x00, 0x00, 0x01)},
		{"routing indicator entirely padding", suciNAS([2]byte{0xff, 0xff}, 0x00, 0x00, 0x01)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := suciFromNAS(tc.buf); ok {
				t.Errorf("accepted an unreadable SUCI and produced %+v; a wrong target identity "+
					"is worse than a missing one", got)
			}
		})
	}
}

// A three-digit MNC decodes differently from a two-digit one, and getting it wrong
// misidentifies the home network rather than failing.
func TestSuciThreeDigitMNC(t *testing.T) {
	buf := []byte{0x01, 0x62, 0x32, 0x54, 0x21, 0xf3, 0x00, 0x00, 0x01}
	got, ok := suciFromNAS(buf)
	if !ok {
		t.Fatal("rejected a SUCI with a three-digit MNC")
	}
	if got.MCC != "262" {
		t.Errorf("MCC = %q, want 262", got.MCC)
	}
	if got.MNC != "453" {
		t.Errorf("MNC = %q, want 453", got.MNC)
	}
}

// Table 8.3.5-1 defines schemeOutput as "the characters resulting as the output of the
// permanent identifier with the protection scheme applied". Under the null scheme
// those characters are the MSIN's digits; NAS carries them nibble-swapped, so shipping
// the raw octets transposes every pair.
func TestSchemeOutputRendersCharactersUnderTheNullScheme(t *testing.T) {
	// The production case: IMSI 208930100007488 is MCC 208, MNC 93, MSIN 0100007488.
	// The MSIN arrives as 10 00 00 47 88 and must be reported as its digits, not as
	// 1000004788 — which is what the octets read as if carried through unchanged.
	got, ok := suciFromNAS(suciNAS([2]byte{0xf0, 0xff}, 0x00, 0x00,
		0x10, 0x00, 0x00, 0x47, 0x88))
	if !ok {
		t.Fatal("rejected a null-scheme SUCI")
	}
	if string(got.SchemeOutput) != "0100007488" {
		t.Errorf("schemeOutput = %q, want \"0100007488\". The raw octets read as "+
			"\"1000004788\" — every digit pair transposed, which is a well-formed record "+
			"naming a subscriber that does not exist", got.SchemeOutput)
	}
}

func TestSchemeOutputOddDigitCount(t *testing.T) {
	// Nine digits: the final high nibble is the 0xf filler and is dropped.
	got, ok := suciFromNAS(suciNAS([2]byte{0xf0, 0xff}, 0x00, 0x00, 0x21, 0x43, 0x65, 0x87, 0xf9))
	if !ok {
		t.Fatal("rejected a null-scheme SUCI with an odd digit count")
	}
	if string(got.SchemeOutput) != "123456789" {
		t.Errorf("schemeOutput = %q, want \"123456789\"", got.SchemeOutput)
	}
}

func TestSchemeOutputPassesCiphertextThrough(t *testing.T) {
	// Profile A/B output is ciphertext: no characters, so the octets stand.
	got, ok := suciFromNAS(suciNAS([2]byte{0xf0, 0xff}, 0x01, 0x1B, 0xDE, 0xAD, 0xBE, 0xEF))
	if !ok {
		t.Fatal("rejected a protected SUCI")
	}
	if !bytes.Equal(got.SchemeOutput, iri.SchemeOutput{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("schemeOutput = %x, want deadbeef unchanged", got.SchemeOutput)
	}
}

func TestSchemeOutputRefusesMisplacedFiller(t *testing.T) {
	// Filler in any octet but the last, or in a low nibble, is not a digit string.
	for _, tc := range []struct {
		name string
		out  []byte
	}{
		{"filler before the final octet", []byte{0x1f, 0x23}},
		{"filler in a low nibble", []byte{0xf1, 0x23}},
		{"empty scheme output", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := suciNAS([2]byte{0xf0, 0xff}, 0x00, 0x00, tc.out...)
			if got, ok := suciFromNAS(buf); ok {
				t.Errorf("accepted %x and produced schemeOutput %q", tc.out, got.SchemeOutput)
			}
		})
	}
}
