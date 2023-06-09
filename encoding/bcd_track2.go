package encoding

import (
	"fmt"

	"github.com/moov-io/iso8583/utils"
	"github.com/yerden/go-util/bcd"
)

var (
	_ Encoder = (*bcdTrack2Encoder)(nil)
	//BCDTrack2         = &bcdTrack2Encoder{}
)

type bcdTrack2Encoder struct{}

func (e *bcdTrack2Encoder) Encode(src []byte) ([]byte, error) {
	if len(src)%2 != 0 {
		src = append([]byte("0"), src...)
	}

	enc := bcd.NewEncoder(bcd.Standard)
	dst := make([]byte, bcd.EncodedLen(len(src)))
	n, err := enc.Encode(dst, src)
	if err != nil {
		return nil, utils.NewSafeError(err, "failed to perform BCD encoding")
	}

	return dst[:n], nil
}

// Returns [true, 0] if separator is in left nibble, [true 1] if separator is in right nibble
// [false -1] if no separator is found
func containsSeparator(b byte) (bool, int) {
	leftNibbleSeparator := byte(0b11010000)  // D separator contained in left nibble
	rightNibbleSeparator := byte(0b00001101) // D separator contained in right nibble

	if res := b & leftNibbleSeparator; res == leftNibbleSeparator {
		//fmt.Printf("found separator in left nibble, byte: %x\n", b) // fresanov
		return true, 0
	}
	if res := b & rightNibbleSeparator; res == rightNibbleSeparator {
		//fmt.Printf("found separator in right nibble, byte: %x\n", b) // fresanov
		return true, 1
	}

	return false, -1
}

// nibble - 0 -> left nibble; 1 -> right nibble
func zeroOutSeparator(b byte, nibble int) byte {
	var result byte
	switch nibble {
	case 0:
		b = b << 4
		result = b >> 4
		return result
	case 1:
		b = b >> 4
		result = b << 4
		return result
	}
	return 0
}

// sometimes last nibble contains bytes in non-BCD range (F for example) - clear them out
func zeroOutLastNibble(b byte) byte {
	//fmt.Printf("zeroing out last byte %x\n", b) // fresanov
	var result byte
	b = b >> 4
	result = b << 4
	//fmt.Printf("zeroing out result %x\n", result) // fresanov
	return result
}

func (e *bcdTrack2Encoder) Decode(src []byte, length int) ([]byte, int, error) {
	// length should be positive
	if length < 0 {
		return nil, 0, fmt.Errorf("length should be positive, got %d", length)
	}

	// because separator is not a digit we extract it in order to avoid BCD decoding error
	var indexOfSeparator int
	var separatorNibble int
	var zeroedOutSeparatorByte byte
	for i, v := range src {
		found, nibble := containsSeparator(v)
		if found {
			indexOfSeparator = i
			// fmt.Printf("index of separator %d\n", indexOfSeparator) // fresanov
			separatorNibble = nibble
			zeroedOutSeparatorByte = zeroOutSeparator(v, nibble)
			break
		}
	}
	//fmt.Printf("separator nibble: %d\n", separatorNibble) // fresanov

	// extracting parts of slice left of separator and right of separator
	// separator byte is not included in either parts

	leftPart := make([]byte, indexOfSeparator)
	copy(leftPart, src[:indexOfSeparator])
	//fmt.Printf("left part %x\n", leftPart) // fresanov

	rightPart := make([]byte, (length/2)-indexOfSeparator-1)
	copy(rightPart, src[indexOfSeparator+1:(length/2)]) // exclude last byte -> we zero out the last nibble in it just in case
	//fmt.Printf("right part %x\n", rightPart) // fresanov

	lastByte := src[(length / 2):]
	//fmt.Printf("last byte slice: %x\n", lastByte) // fresanov
	zeroedOutLastByte := zeroOutLastNibble(lastByte[0])
	rightPart = append(rightPart, zeroedOutLastByte)

	// 'fake' zeroed out separator byte is inserted in a newly creatd slice which is then decoded
	// later we swap the byte with the one containing the 'non-BCD' (D) separator
	corrected := make([]byte, 0)
	corrected = append(corrected, leftPart...)
	corrected = append(corrected, zeroedOutSeparatorByte)
	corrected = append(corrected, rightPart...)

	// for BCD encoding the length should be even
	decodedLen := length
	if length%2 != 0 {
		decodedLen += 1
	}

	// how many bytes we will read
	read := bcd.EncodedLen(decodedLen)

	if len(corrected) < read {
		return nil, 0, fmt.Errorf("not enough data to decode. expected len %d, got %d", read, len(src))
	}

	dec := bcd.NewDecoder(bcd.Standard)
	dst := make([]byte, decodedLen)
	//fmt.Printf("calling decode with arg: %x\n", corrected[:read]) // fresanov
	_, err := dec.Decode(dst, corrected[:read])
	if err != nil {
		//fmt.Printf("bcd errore: %v\n", err) // fresanov
		return nil, 0, utils.NewSafeError(err, "failed to perform BCD decoding")
	}
	//fmt.Printf("decoded track2: %x\n", dst) // fresanov
	//fmt.Printf("decoded track2 string: %s\n", dst) // fresanov

	trackString := string(dst)
	//fmt.Printf("trackString converted: %s\n", trackString) // fresanov
	var separatorPosition int
	if separatorNibble == 0 {
		separatorPosition = (len(leftPart) * 2)
	}
	if separatorNibble == 1 {
		separatorPosition = (len(leftPart) * 2) + 1
	}
	trackString = replaceAtIndex(trackString, '=', separatorPosition)
	//fmt.Printf("trackString replaced: %s\n", trackString) // fresanov

	result := []byte(trackString)

	return result, read, nil
}

func replaceAtIndex(in string, r rune, i int) string {
	out := []rune(in)
	out[i] = r
	return string(out)
}
