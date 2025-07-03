// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package aper

import (
	"fmt"
	"log"
	"reflect"
)

type perRawBitData struct {
	bytes      []byte
	bitsOffset uint
}

func perRawBitLog(numBits uint64, byteLen int, bitsOffset uint, value interface{}) string {
	if reflect.TypeOf(value).Kind() == reflect.Uint64 {
		return fmt.Sprintf("  [PER put %2d bits, byteLen(after): %d, bitsOffset(after): %d, value: 0x%0x]",
			numBits, byteLen, bitsOffset, reflect.ValueOf(value).Uint())
	}
	return fmt.Sprintf("  [PER put %2d bits, byteLen(after): %d, bitsOffset(after): %d, value: 0x%0x]",
		numBits, byteLen, bitsOffset, reflect.ValueOf(value).Bytes())
}

func (pd *perRawBitData) bitCarry() {
	pd.bitsOffset = pd.bitsOffset & 0x07
}

func (pd *perRawBitData) appendAlignBits() {
	if alignBits := uint64(8-pd.bitsOffset&0x7) & 0x7; alignBits != 0 {
		perTrace(2, fmt.Sprintf("Aligning %d bits", alignBits))
		perTrace(1, perRawBitLog(alignBits, len(pd.bytes), 0, []byte{0x00}))
	}
	pd.bitsOffset = 0
}

func (pd *perRawBitData) putBitString(bytes []byte, numBits uint) error {
	var err error

	bytes = bytes[:(numBits+7)>>3]
	if pd.bitsOffset == 0 {
		pd.bytes = append(pd.bytes, bytes...)
		pd.bitsOffset = (numBits & 0x7)
		perTrace(1, perRawBitLog(uint64(numBits), len(pd.bytes), pd.bitsOffset, bytes))
		return err
	}
	bitsLeft := 8 - pd.bitsOffset
	currentByte := len(pd.bytes) - 1
	if numBits <= bitsLeft {
		pd.bytes[currentByte] |= (bytes[0] >> pd.bitsOffset)
	} else {
		bytes = append([]byte{0x00}, bytes...)
		var shiftBytes []byte
		if shiftBytes, err = GetBitString(bytes, bitsLeft, pd.bitsOffset+numBits); err != nil {
			return err
		}
		pd.bytes[currentByte] |= shiftBytes[0]
		pd.bytes = append(pd.bytes, shiftBytes[1:]...)
		bytes = bytes[1:]
	}
	pd.bitsOffset = (numBits & 0x7) + pd.bitsOffset
	pd.bitCarry()
	perTrace(1, perRawBitLog(uint64(numBits), len(pd.bytes), pd.bitsOffset, bytes))
	return err
}

func (pd *perRawBitData) putBitsValue(value uint64, numBits uint) error {
	var err error
	if numBits == 0 {
		return err
	}
	Byteslen := (numBits + 7) >> 3
	tempBytes := make([]byte, Byteslen)
	bitOff := numBits & 0x7
	if bitOff == 0 {
		bitOff = 8
	}
	LeftbitOff := 8 - bitOff
	tempBytes[Byteslen-1] = byte((value << LeftbitOff) & 0xff)
	value >>= bitOff
	var i int
	for i = int(Byteslen) - 2; value > 0; i-- {
		if i < 0 {
			return fmt.Errorf("bits Value is over capacity")
		}
		tempBytes[i] = byte(value & 0xff)
		value >>= 8
	}

	err = pd.putBitString(tempBytes, numBits)
	return err
}

func (pd *perRawBitData) appendConstraintValue(valueRange int64, value uint64) error {
	var err error
	perTrace(3, fmt.Sprintf("Putting Constraint Value %d with range %d", value, valueRange))

	var bytes uint
	if valueRange <= 255 {
		if valueRange < 0 {
			return fmt.Errorf("value range is negative")
		}
		var i uint
		// 1 ~ 8 bits
		for i = 1; i <= 8; i++ {
			upper := 1 << i
			if int64(upper) >= valueRange {
				break
			}
		}
		err = pd.putBitsValue(value, i)
		return err
	} else if valueRange == 256 {
		bytes = 1
	} else if valueRange <= 65536 {
		bytes = 2
	} else {
		return fmt.Errorf("constraint Value is large than 65536")
	}
	pd.appendAlignBits()
	err = pd.putBitsValue(value, bytes*8)
	return err
}

func (pd *perRawBitData) appendNormallySmallNonNegativeValue(value uint64) error {
	var err error
	perTrace(3, fmt.Sprintf("Putting Normally Small Non-Negative Value %d", value))

	if value < 64 {
		if err = pd.putBitsValue(0, 1); err != nil {
			return err
		}
		err = pd.putBitsValue(value, 6)
	} else {
		if err = pd.putBitsValue(1, 1); err != nil {
			return err
		}
		err = pd.putSemiConstrainedWholeNumber(value, 0)
	}
	return err
}

func (pd *perRawBitData) putSemiConstrainedWholeNumber(value uint64, lb uint64) error {
	if lb > value {
		return fmt.Errorf("value (%d) is less than lower bound value (%d)", value, lb)
	}
	value -= lb
	var length uint64 = 1
	valueTmp := value >> 8
	for valueTmp > 0 {
		length++
		valueTmp = valueTmp >> 8
	}
	if err := pd.appendLength(-1, length); err != nil {
		return err
	}
	if err := pd.putBitsValue(value, uint(length)*8); err != nil {
		return err
	}
	return nil
}

func (pd *perRawBitData) appendLength(sizeRange int64, value uint64) (err error) {
	if sizeRange <= 65536 && sizeRange > 0 {
		return pd.appendConstraintValue(sizeRange, value)
	}
	pd.appendAlignBits()
	perTrace(2, fmt.Sprintf("Putting Length of Value : %d", value))
	if value <= 127 {
		err = pd.putBitsValue(value, 8)
		return
	} else if value <= 16383 {
		value |= 0x8000
		err = pd.putBitsValue(value, 16)
		return
	}

	value = (value >> 14) | 0xc0
	err = pd.putBitsValue(value, 8)
	return
}

func (pd *perRawBitData) appendBitString(bytes []byte, bitsLength uint64, extensive bool,
	lowerBoundPtr *int64, upperBoundPtr *int64,
) error {
	var err error
	var lb, ub, sizeRange int64 = 0, -1, -1
	if lowerBoundPtr != nil {
		lb = *lowerBoundPtr
		if upperBoundPtr != nil {
			ub = *upperBoundPtr
			if bitsLength <= uint64(ub) {
				sizeRange = ub - lb + 1
			} else if !extensive {
				return fmt.Errorf("bitString Length is over upperbound")
			}
			if extensive {
				perTrace(2, "Putting size Extension Value")
				if sizeRange == -1 {
					if errTmp := pd.putBitsValue(1, 1); errTmp != nil {
						log.Printf("putBitsValue(1, 1) error: %v", errTmp)
					}
					lb = 0
				} else {
					if errTmp := pd.putBitsValue(0, 1); errTmp != nil {
						log.Printf("putBitsValue(0, 1) error: %v", errTmp)
					}
				}
			}
		}
	}

	if ub > 65535 {
		sizeRange = -1
	}
	sizes := (bitsLength + 7) >> 3
	shift := (8 - bitsLength&0x7)
	if shift != 8 {
		bytes[sizes-1] &= (0xff << shift)
	}

	if sizeRange == 1 {
		if bitsLength != uint64(ub) {
			err = fmt.Errorf("bitString Length(%d) is not match fix-sized : %d", bitsLength, ub)
		}
		perTrace(2, fmt.Sprintf("Encoding BIT STRING size %d", ub))
		if sizes > 2 {
			pd.appendAlignBits()
			pd.bytes = append(pd.bytes, bytes...)
			pd.bitsOffset = uint(ub & 0x7)
			perTrace(1, perRawBitLog(bitsLength, len(pd.bytes), pd.bitsOffset, bytes))
		} else {
			err = pd.putBitString(bytes, uint(bitsLength))
		}
		perTrace(2, fmt.Sprintf("Encoded BIT STRING (length = %d): 0x%0x", bitsLength, bytes))
		return err
	}
	rawLength := bitsLength - uint64(lb)

	var byteOffset, partOfRawLength uint64
	for {
		if rawLength > 65536 {
			partOfRawLength = 65536
		} else if rawLength >= 16384 {
			partOfRawLength = rawLength & 0xc000
		} else {
			partOfRawLength = rawLength
		}
		if err = pd.appendLength(sizeRange, partOfRawLength); err != nil {
			return err
		}
		partOfRawLength += uint64(lb)
		sizes := (partOfRawLength + 7) >> 3
		perTrace(2, fmt.Sprintf("Encoding BIT STRING size %d", partOfRawLength))
		if partOfRawLength == 0 {
			return err
		}
		pd.appendAlignBits()
		pd.bytes = append(pd.bytes, bytes[byteOffset:byteOffset+sizes]...)
		perTrace(1, perRawBitLog(partOfRawLength, len(pd.bytes), pd.bitsOffset, bytes))
		perTrace(2, fmt.Sprintf("Encoded BIT STRING (length = %d): 0x%0x", partOfRawLength,
			bytes[byteOffset:byteOffset+sizes]))
		rawLength -= (partOfRawLength - uint64(lb))
		if rawLength > 0 {
			byteOffset += sizes
		} else {
			pd.bitsOffset += uint(partOfRawLength & 0x7)
			// pd.appendAlignBits()
			break
		}
	}
	return err
}

func (pd *perRawBitData) appendOctetString(bytes []byte, extensive bool, lowerBoundPtr *int64,
	upperBoundPtr *int64,
) error {
	byteLen := uint64(len(bytes))
	var lb, ub, sizeRange int64 = 0, -1, -1
	if lowerBoundPtr != nil {
		lb = *lowerBoundPtr
		if upperBoundPtr != nil {
			ub = *upperBoundPtr
			if byteLen <= uint64(ub) {
				sizeRange = ub - lb + 1
			} else if !extensive {
				return fmt.Errorf("octetString Length is over upperbound")
			}
			if extensive {
				perTrace(2, "Putting size Extension Value")
				if sizeRange == -1 {
					if errTmp := pd.putBitsValue(1, 1); errTmp != nil {
						log.Printf("putBitsValue(1, 1) err: %v", errTmp)
					}
					lb = 0
				} else {
					if errTmp := pd.putBitsValue(0, 1); errTmp != nil {
						log.Printf("putBitsValue(0, 1) err: %v", errTmp)
					}
				}
			}
		}
	}

	if ub > 65535 {
		sizeRange = -1
	}

	if sizeRange == 1 {
		if byteLen != uint64(ub) {
			return fmt.Errorf("octetString length (%d) is not match fix-sized: %d", byteLen, ub)
		}
		perTrace(2, fmt.Sprintf("Encoding OCTET STRING size %d", ub))
		if byteLen > 2 {
			pd.appendAlignBits()
			pd.bytes = append(pd.bytes, bytes...)
			perTrace(1, perRawBitLog(byteLen*8, len(pd.bytes), 0, bytes))
		} else {
			err := pd.putBitString(bytes, uint(byteLen*8))
			return err
		}
		perTrace(2, fmt.Sprintf("Encoded OCTET STRING (length = %d): 0x%0x", byteLen, bytes))
		return nil
	}
	rawLength := byteLen - uint64(lb)

	var byteOffset, partOfRawLength uint64
	for {
		if rawLength > 65536 {
			partOfRawLength = 65536
		} else if rawLength >= 16384 {
			partOfRawLength = rawLength & 0xc000
		} else {
			partOfRawLength = rawLength
		}
		if err := pd.appendLength(sizeRange, partOfRawLength); err != nil {
			return err
		}
		partOfRawLength += uint64(lb)
		perTrace(2, fmt.Sprintf("Encoding OCTET STRING size %d", partOfRawLength))
		if partOfRawLength == 0 {
			return nil
		}
		pd.appendAlignBits()
		pd.bytes = append(pd.bytes, bytes[byteOffset:byteOffset+partOfRawLength]...)
		perTrace(1, perRawBitLog(partOfRawLength*8, len(pd.bytes), pd.bitsOffset, bytes))
		perTrace(2, fmt.Sprintf("Encoded OCTET STRING (length = %d): 0x%0x", partOfRawLength,
			bytes[byteOffset:byteOffset+partOfRawLength]))
		rawLength -= (partOfRawLength - uint64(lb))
		if rawLength > 0 {
			byteOffset += partOfRawLength
		} else {
			// pd.appendAlignBits()
			break
		}
	}
	return nil
}

func (pd *perRawBitData) appendBool(value bool) error {
	var err error
	perTrace(3, fmt.Sprintf("Encoding BOOLEAN Value %t", value))
	if value {
		err = pd.putBitsValue(1, 1)
		perTrace(2, "Encoded BOOLEAN Value : 0x1")
	} else {
		err = pd.putBitsValue(0, 1)
		perTrace(2, "Encoded BOOLEAN Value : 0x0")
	}
	return err
}

func (pd *perRawBitData) appendInteger(value int64, extensive bool, lowerBoundPtr *int64, upperBoundPtr *int64) error {
	var lb, valueRange int64 = 0, 0
	if lowerBoundPtr != nil {
		lb = *lowerBoundPtr
		if value < lb {
			return fmt.Errorf("integer value is smaller than lowerbound")
		}
		if upperBoundPtr != nil {
			ub := *upperBoundPtr
			if value <= ub {
				valueRange = ub - lb + 1
			} else if !extensive {
				return fmt.Errorf("integer value is larger than upperbound")
			}
			if extensive {
				perTrace(2, "Putting value Extension bit")
				if valueRange == 0 {
					perTrace(3, "Encoding INTEGER with Unconstraint Value")
					valueRange = -1
					if errTmp := pd.putBitsValue(1, 1); errTmp != nil {
						fmt.Printf("pd.putBitsValue(1, 1) error: %v", errTmp)
					}
				} else {
					perTrace(3, fmt.Sprintf("Encoding INTEGER with Value Range(%d..%d)", lb, ub))
					if errTmp := pd.putBitsValue(0, 1); errTmp != nil {
						fmt.Printf("pd.putBitsValue(0, 1) error: %v", errTmp)
					}
				}
			}
		} else {
			perTrace(3, fmt.Sprintf("Encoding INTEGER with Semi-Constraint Range(%d..)", lb))
		}
	} else {
		perTrace(3, "Encoding INTEGER with Unconstraint Value")
		valueRange = -1
	}

	unsignedValue := uint64(value)
	var rawLength uint
	if valueRange == 1 {
		perTrace(2, "Value of INTEGER is fixed")

		return nil
	}
	if value < 0 {
		y := value >> 63
		unsignedValue = uint64(((value ^ y) - y)) - 1
	}
	if valueRange <= 0 {
		unsignedValue >>= 7
	} else if valueRange <= 65536 {
		return pd.appendConstraintValue(valueRange, uint64(value-lb))
	} else {
		unsignedValue >>= 8
	}
	for rawLength = 1; rawLength <= 127; rawLength++ {
		if unsignedValue == 0 {
			break
		}
		unsignedValue >>= 8
	}
	// putting length
	if valueRange <= 0 {
		// semi-constraint or unconstraint
		pd.appendAlignBits()
		pd.bytes = append(pd.bytes, byte(rawLength))
		perTrace(2, fmt.Sprintf("Encoding INTEGER Length %d in one byte", rawLength))

		perTrace(1, perRawBitLog(8, len(pd.bytes), pd.bitsOffset, uint64(rawLength)))
	} else {
		// valueRange > 65536
		var byteLen uint
		unsignedValueRange := uint64(valueRange - 1)
		for byteLen = 1; byteLen <= 127; byteLen++ {
			unsignedValueRange >>= 8
			if unsignedValueRange <= 1 {
				break
			}
		}
		var i, upper uint
		// 1 ~ 8 bits
		for i = 1; i <= 8; i++ {
			upper = 1 << i
			if upper >= byteLen {
				break
			}
		}
		perTrace(2, fmt.Sprintf("Encoding INTEGER Length %d-1 in %d bits", rawLength, i))
		if err := pd.putBitsValue(uint64(rawLength-1), i); err != nil {
			return err
		}
	}
	perTrace(2, fmt.Sprintf("Encoding INTEGER %d with %d bytes", value, rawLength))

	rawLength *= 8
	pd.appendAlignBits()

	if valueRange < 0 {
		mask := int64(1<<rawLength - 1)
		return pd.putBitsValue(uint64(value&mask), rawLength)
	} else {
		value -= lb
		return pd.putBitsValue(uint64(value), rawLength)
	}
}

func (pd *perRawBitData) appendEnumerated(value uint64, extensive bool, lowerBoundPtr *int64,
	upperBoundPtr *int64,
) error {
	if lowerBoundPtr == nil || upperBoundPtr == nil {
		return fmt.Errorf("enumerated value constraint is error")
	}
	lb, ub := *lowerBoundPtr, *upperBoundPtr
	if lb < 0 || lb > ub {
		return fmt.Errorf("enumerated value constraint is error")
	}

	if value <= uint64(ub) {
		if extensive {
			if err := pd.putBitsValue(0, 1); err != nil {
				return err
			}
		}
		valueRange := ub - lb + 1
		perTrace(2, fmt.Sprintf("Encoding ENUMERATED Value : %d with Value Range(%d..%d)", value, lb, ub))
		if valueRange > 1 {
			return pd.appendConstraintValue(valueRange, value)
		}
	} else {
		if !extensive {
			return fmt.Errorf("enumerated value is larger than upperbound")
		}
		if err := pd.putBitsValue(1, 1); err != nil {
			return err
		}
		return pd.appendNormallySmallNonNegativeValue(value - uint64(ub) - 1)
	}

	return nil
}

func (pd *perRawBitData) parseSequenceOf(v reflect.Value, params fieldParameters) error {
	var lb, ub, sizeRange int64 = 0, -1, -1
	numElements := int64(v.Len())
	if params.sizeLowerBound != nil && *params.sizeLowerBound < 65536 {
		lb = *params.sizeLowerBound
	}
	if params.sizeUpperBound != nil && *params.sizeUpperBound < 65536 {
		ub = *params.sizeUpperBound
		if params.sizeExtensible {
			if numElements > ub {
				if err := pd.putBitsValue(1, 1); err != nil {
					return err
				}
			} else {
				if err := pd.putBitsValue(0, 1); err != nil {
					return err
				}
				sizeRange = ub - lb + 1
			}
		} else if numElements > ub {
			return fmt.Errorf("sequence of size is larger than upperbound")
		} else {
			sizeRange = ub - lb + 1
		}
	} else {
		sizeRange = -1
	}

	if numElements < lb {
		return fmt.Errorf("sequence of size is lower than lowerbound")
	} else if sizeRange == 1 {
		perTrace(3, fmt.Sprintf("Encoding Length of \"SEQUENCE OF\"  with fix-size %d", ub))
		if numElements != ub {
			return fmt.Errorf("encoding length %d != fix-size %d", numElements, ub)
		}
	} else if sizeRange > 0 {
		perTrace(3, fmt.Sprintf("Encoding Length(%d) of \"SEQUENCE OF\"  with Size Range(%d..%d)", numElements, lb, ub))
		if err := pd.appendConstraintValue(sizeRange, uint64(numElements-lb)); err != nil {
			return err
		}
	} else {
		perTrace(3, fmt.Sprintf("Encoding Length(%d) of \"SEQUENCE OF\" with Semi-Constraint Range(%d..)", numElements, lb))
		pd.appendAlignBits()
		pd.bytes = append(pd.bytes, byte(numElements&0xff))
		perTrace(1, perRawBitLog(8, len(pd.bytes), pd.bitsOffset, uint64(numElements)))
	}
	perTrace(2, fmt.Sprintf("Encoding  \"SEQUENCE OF\" struct %s with len(%d)", v.Type().Elem().Name(), numElements))
	params.sizeExtensible = false
	params.sizeUpperBound = nil
	params.sizeLowerBound = nil
	for i := 0; i < v.Len(); i++ {
		if err := pd.makeField(v.Index(i), params); err != nil {
			return err
		}
	}
	return nil
}

func (pd *perRawBitData) appendChoiceIndex(present int, extensive bool, upperBoundPtr *int64) error {
	var ub int64
	rawChoice := present - 1
	if upperBoundPtr == nil {
		return fmt.Errorf("the upper bound of CHIOCE is missing")
	} else if ub = *upperBoundPtr; ub < 0 {
		return fmt.Errorf("the upper bound of CHIOCE is negative")
	} else if extensive && rawChoice > int(ub) {
		return fmt.Errorf("unsupport value of CHOICE type is in Extensed")
	}
	perTrace(2, fmt.Sprintf("Encoding Present index of CHOICE  %d - 1", present))
	if err := pd.appendConstraintValue(ub+1, uint64(rawChoice)); err != nil {
		return err
	}
	return nil
}

func (pd *perRawBitData) appendOpenType(v reflect.Value, params fieldParameters) error {
	pdOpenType := &perRawBitData{[]byte(""), 0}
	perTrace(2, fmt.Sprintf("Encoding OpenType %s to temp RawData", v.Type().String()))
	if err := pdOpenType.makeField(v, params); err != nil {
		return err
	}
	openTypeBytes := pdOpenType.bytes
	rawLength := uint64(len(pdOpenType.bytes))
	perTrace(2, fmt.Sprintf("Encoding OpenType %s RawData : 0x%0x(%d bytes)", v.Type().String(), pdOpenType.bytes,
		rawLength))

	var byteOffset, partOfRawLength uint64
	for {
		if rawLength > 65536 {
			partOfRawLength = 65536
		} else if rawLength >= 16384 {
			partOfRawLength = rawLength & 0xc000
		} else {
			partOfRawLength = rawLength
		}
		if err := pd.appendLength(-1, partOfRawLength); err != nil {
			return err
		}
		perTrace(2, fmt.Sprintf("Encoding Part of OpenType RawData size %d", partOfRawLength))
		if partOfRawLength == 0 {
			return nil
		}
		pd.appendAlignBits()
		pd.bytes = append(pd.bytes, openTypeBytes[byteOffset:byteOffset+partOfRawLength]...)
		perTrace(1, perRawBitLog(partOfRawLength*8, len(pd.bytes), pd.bitsOffset, openTypeBytes))
		perTrace(2, fmt.Sprintf("Encoded OpenType RawData (length = %d): 0x%0x", partOfRawLength,
			openTypeBytes[byteOffset:byteOffset+partOfRawLength]))
		rawLength -= partOfRawLength
		if rawLength > 0 {
			byteOffset += partOfRawLength
		} else {
			pd.appendAlignBits()
			break
		}
	}

	perTrace(2, fmt.Sprintf("Encoded OpenType %s", v.Type().String()))
	return nil
}

func (pd *perRawBitData) makeField(v reflect.Value, params fieldParameters) error {
	if !v.IsValid() {
		return fmt.Errorf("aper: cannot marshal nil value")
	}
	// If the field is an interface{} then recurse into it.
	if v.Kind() == reflect.Interface && v.Type().NumMethod() == 0 {
		return pd.makeField(v.Elem(), params)
	}
	if v.Kind() == reflect.Ptr {
		return pd.makeField(v.Elem(), params)
	}
	fieldType := v.Type()

	// We deal with the structures defined in this package first.
	switch fieldType {
	case BitStringType:
		err := pd.appendBitString(v.Field(0).Bytes(), v.Field(1).Uint(), params.sizeExtensible, params.sizeLowerBound,
			params.sizeUpperBound)
		return err
	case ObjectIdentifierType:
		return fmt.Errorf("unsupport ObjectIdenfier type")
	case OctetStringType:
		err := pd.appendOctetString(v.Bytes(), params.sizeExtensible, params.sizeLowerBound, params.sizeUpperBound)
		return err
	case EnumeratedType:
		err := pd.appendEnumerated(v.Uint(), params.valueExtensible, params.valueLowerBound, params.valueUpperBound)
		return err
	}
	switch val := v; val.Kind() {
	case reflect.Bool:
		err := pd.appendBool(v.Bool())
		return err
	case reflect.Int, reflect.Int32, reflect.Int64:
		err := pd.appendInteger(v.Int(), params.valueExtensible, params.valueLowerBound, params.valueUpperBound)
		return err

	case reflect.Struct:

		structType := fieldType
		var structParams []fieldParameters
		var optionalCount uint
		var optionalPresents uint64
		var sequenceType bool
		// struct extensive TODO: support extensed type
		if params.valueExtensible {
			perTrace(2, fmt.Sprintf("Encoding Value Extensive Bit : %t", false))
			if err := pd.putBitsValue(0, 1); err != nil {
				return err
			}
		}
		sequenceType = (structType.NumField() <= 0 || structType.Field(0).Name != PRESENT)
		// pass tag for optional
		for i := 0; i < structType.NumField(); i++ {
			if structType.Field(i).PkgPath != "" {
				return fmt.Errorf("struct contains unexported fields : %s", structType.Field(i).PkgPath)
			}
			tempParams := parseFieldParameters(structType.Field(i).Tag.Get("aper"))
			if sequenceType {
				// for optional flag
				if tempParams.optional {
					optionalCount++
					optionalPresents <<= 1
					if !v.Field(i).IsNil() {
						optionalPresents++
					}
				} else if v.Field(i).Type().Kind() == reflect.Ptr && v.Field(i).IsNil() {
					return fmt.Errorf("nil element in SEQUENCE type")
				}
			}

			structParams = append(structParams, tempParams)
		}
		if optionalCount > 0 {
			perTrace(2, fmt.Sprintf("putting optional(%d), optionalPresents is %0b", optionalCount, optionalPresents))
			if err := pd.putBitsValue(optionalPresents, optionalCount); err != nil {
				return err
			}
		}

		// CHOICE or OpenType
		if !sequenceType {
			present := int(v.Field(0).Int())
			if present == 0 {
				return fmt.Errorf("choice or OpenType present is 0 (present's field number)")
			} else if present >= structType.NumField() {
				return fmt.Errorf("present is bigger than number of struct field")
			} else if params.openType {
				if params.referenceFieldValue == nil {
					return fmt.Errorf("openType reference value is empty")
				}
				refValue := *params.referenceFieldValue

				if structParams[present].referenceFieldValue == nil || *structParams[present].referenceFieldValue != refValue {
					return fmt.Errorf("reference value and present reference value is not match")
				}
				perTrace(2, fmt.Sprintf("Encoding Present index of OpenType is %d ", present))
				if err := pd.appendOpenType(val.Field(present), structParams[present]); err != nil {
					return err
				}
			} else {
				if err := pd.appendChoiceIndex(present, params.valueExtensible, params.valueUpperBound); err != nil {
					return err
				}
				if err := pd.makeField(val.Field(present), structParams[present]); err != nil {
					return err
				}
			}
			return nil
		}

		for i := 0; i < structType.NumField(); i++ {
			// optional
			if structParams[i].optional && optionalCount > 0 {
				optionalCount--
				if optionalPresents&(1<<optionalCount) == 0 {
					perTrace(3, fmt.Sprintf("Field \"%s\" in %s is OPTIONAL and not present", structType.Field(i).Name, structType))
					continue
				} else {
					perTrace(3, fmt.Sprintf("Field \"%s\" in %s is OPTIONAL and present", structType.Field(i).Name, structType))
				}
			}
			// for open type reference
			if structParams[i].openType {
				fieldName := structParams[i].referenceFieldName
				var index int
				for index = 0; index < i; index++ {
					if structType.Field(index).Name == fieldName {
						break
					}
				}
				if index == i {
					return fmt.Errorf("open type is not reference to the other field in the struct")
				}
				structParams[i].referenceFieldValue = new(int64)
				if value, err := getReferenceFieldValue(val.Field(index)); err != nil {
					return err
				} else {
					*structParams[i].referenceFieldValue = value
				}
			}
			if err := pd.makeField(val.Field(i), structParams[i]); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice:
		err := pd.parseSequenceOf(v, params)
		return err
	case reflect.String:
		printableString := v.String()
		perTrace(2, fmt.Sprintf("Encoding PrintableString : \"%s\" using Octet String decoding method", printableString))
		err := pd.appendOctetString([]byte(printableString), params.sizeExtensible, params.sizeLowerBound,
			params.sizeUpperBound)
		return err
	}
	return fmt.Errorf("unsupported: %s", v.Type().String())
}

// Marshal returns the ASN.1 encoding of val.
func Marshal(val interface{}) ([]byte, error) {
	return MarshalWithParams(val, "")
}

// MarshalWithParams allows field parameters to be specified for the
// top-level element. The form of the params is the same as the field tags.
func MarshalWithParams(val interface{}, params string) ([]byte, error) {
	pd := &perRawBitData{[]byte(""), 0}
	err := pd.makeField(reflect.ValueOf(val), parseFieldParameters(params))
	if err != nil {
		return nil, err
	} else if len(pd.bytes) == 0 {
		pd.bytes = make([]byte, 1)
	}
	return pd.bytes, nil
}
