// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"encoding/hex"
	"math/bits"
	"strconv"
	"strings"

	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/util/li/iri"
)

// suciFromNAS builds a TS 33.128 SUCI record member from the raw 5GS mobile
// identity octets the UE sent, and reports whether it could.
//
// It reads the octets rather than AmfUe.Suci, which is the string
// nasConvert.SuciToString produced —
// "suci-0-<mcc>-<mnc>-<routingInd>-<scheme>-<hnpkid>-<schemeOutput>". Parsing that
// string back into the record's members is lossy in three places, each of which
// yields a well-formed record naming the wrong subscriber:
//
//   - the routing indicator has had its 'f' padding stripped, so "0123" and "123"
//     are indistinguishable once parsed as an integer — which is exactly what the
//     module's routingIndicatorLength exists to disambiguate;
//   - homeNetworkPublicKeyIdentifier is formatted with %d while the ASN.1 type is an
//     OCTET STRING;
//   - schemeOutput under the null scheme is nibble-swapped MSIN with a trailing 'f'
//     removed, so it can be an odd number of hex digits and will not decode.
//
// Reading the octets removes the first two. It does *not* remove the third, which is
// the one that matters most: table 8.3.5-1 defines schemeOutput as "the characters
// resulting as the output of the permanent identifier with the protection scheme
// applied", and under the null scheme those characters are the MSIN's digits. The raw
// octets are that MSIN in nibble-swapped BCD, so shipping them transposes every pair —
// MSIN 0100007488 goes out as 1000004788, a well-formed record naming a subscriber
// that does not exist. See schemeOutput below.
//
// The rule throughout is that a SUCI which does not yield clean members produces no
// sUCI at all. A missing target identity is a gap an audit can find; a wrong one is
// a record an agency will act on.
//
// Layout, per TS 24.501 clause 9.11.3.4:
//
//	[0]    SUPI format (bits 5-7) and type of identity (bits 1-3)
//	[1..3] MCC/MNC, BCD
//	[4..5] routing indicator, BCD, 'f'-padded
//	[6]    protection scheme identifier (low nibble)
//	[7]    home network public key identifier
//	[8..]  scheme output
func suciFromNAS(buf []byte) (iri.SUCI, bool) {
	// Nine octets is the shortest well-formed SUCI: everything up to and including
	// the first octet of scheme output.
	if len(buf) < 9 {
		return iri.SUCI{}, false
	}
	if buf[0]&0x07 != nasMessage.MobileIdentity5GSTypeSuci {
		return iri.SUCI{}, false
	}
	// The NAI format carries a network-specific identifier rather than the
	// MCC/MNC/routing-indicator structure SUCI ::= SEQUENCE requires, so there is
	// nothing to build. Reported absent rather than approximated.
	if (buf[0]&0xf0)>>4 != nasMessage.SupiFormatImsi {
		return iri.SUCI{}, false
	}

	mcc, mnc, ok := plmnFromNAS(buf[1], buf[2], buf[3])
	if !ok {
		return iri.SUCI{}, false
	}

	digits := bcdDigits(buf[4], buf[5])
	if digits == "" {
		return iri.SUCI{}, false
	}
	routing, err := strconv.Atoi(digits)
	if err != nil {
		return iri.SUCI{}, false
	}

	output, ok := schemeOutput(buf[6]&0x0f, buf[8:])
	if !ok {
		return iri.SUCI{}, false
	}

	suci := iri.SUCI{
		MCC:                    iri.MCC(mcc),
		MNC:                    iri.MNC(mnc),
		RoutingIndicator:       iri.RoutingIndicator(routing),
		ProtectionSchemeID:     iri.ProtectionSchemeID(buf[6] & 0x0f),
		HomeNetworkPublicKeyID: iri.HomeNetworkPublicKeyID{buf[7]},
		SchemeOutput:           output,
	}

	// "shall be included if different from the number of meaningful digits given in
	// routingIndicator". A leading zero is the whole of the difference: 4 digits of
	// "0123" arrive as the integer 123, and without the length the mediation
	// function cannot recover the identifier the UE actually sent.
	if n := len(digits); n != len(strconv.Itoa(routing)) {
		length := iri.RoutingIndicatorLength(n)
		suci.RoutingIndicatorLength = &length
	}

	return suci, true
}

// plmnFromNAS decodes the three BCD octets carrying MCC and MNC. The nibbles are
// stored low-digit-first, so each octet is swapped before being read as hex.
func plmnFromNAS(o1, o2, o3 byte) (mcc, mnc string, ok bool) {
	mcc = hex.EncodeToString([]byte{bits.RotateLeft8(o1, 4), (o2 & 0x0f) << 4})[:3]
	mncDigits := hex.EncodeToString([]byte{bits.RotateLeft8(o3, 4), ((o2 & 0xf0) >> 4) << 4})
	if mncDigits[2] == 'f' {
		mnc = mncDigits[:2] // two-digit MNC, third position filled
	} else {
		mnc = mncDigits[:3]
	}
	// NumericString admits digits only, and li/iri's constraint tables check the
	// alphabet as well as the length — so a filler nibble that reached here would be
	// refused at encode time with the record already built. Cheaper to notice now.
	if strings.ContainsFunc(mcc+mnc, func(r rune) bool { return r < '0' || r > '9' }) {
		return "", "", false
	}

	return mcc, mnc, true
}

// bcdDigits reads two BCD octets as up to four digits, stopping at the 'f' padding
// TS 23.003 uses for a routing indicator shorter than four digits.
func bcdDigits(o1, o2 byte) string {
	s := hex.EncodeToString([]byte{bits.RotateLeft8(o1, 4), bits.RotateLeft8(o2, 4)})
	if i := strings.IndexByte(s, 'f'); i != -1 {
		// Padding runs to the end once it starts. A digit after it means these octets
		// are not a routing indicator, and truncating at the first 'f' would silently
		// accept "1f23" as the single digit 1.
		if strings.Trim(s[i:], "f") != "" {
			return ""
		}
		s = s[:i]
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}

	return s
}

// schemeOutput renders the concealed identifier the way table 8.3.5-1 describes it:
// "the characters resulting as the output of the permanent identifier with the
// protection scheme applied".
//
// Under the null scheme (0) that output is the MSIN, carried in NAS as nibble-swapped
// BCD, and the characters are its digits — so the nibbles are unswapped and rendered
// as ASCII. Shipping the raw octets instead transposes every digit pair, which is a
// well-formed record naming a different subscriber.
//
// Under any other scheme the output is ciphertext with no characters to speak of, so
// the octets are carried through unchanged.
func schemeOutput(scheme byte, raw []byte) (iri.SchemeOutput, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	if int(scheme) != nasMessage.ProtectionSchemeNullScheme {
		return append(iri.SchemeOutput(nil), raw...), true
	}

	digits := make(iri.SchemeOutput, 0, len(raw)*2)
	for i, o := range raw {
		lo, hi := o&0x0f, o>>4
		if lo > 9 {
			return nil, false // filler cannot start an octet
		}
		digits = append(digits, '0'+lo)
		if hi == 0x0f {
			// The odd-length filler, valid only in the final octet.
			if i != len(raw)-1 {
				return nil, false
			}

			return digits, true
		}
		if hi > 9 {
			return nil, false
		}
		digits = append(digits, '0'+hi)
	}

	return digits, true
}
