//go:build go1.27 && goexperiment.simd

package redglob

import (
	"simd"
	"unicode/utf8"
	"unsafe"
)

const minSIMDFoldLength = 64

func matchASCIIFoldPrefix(str, pattern string) (int, bool) {
	if min(len(str), len(pattern)) < minSIMDFoldLength {
		return matchASCIIFoldPrefixScalar(str, pattern)
	}
	return matchASCIIFoldPrefixSIMD(str, pattern)
}

func matchASCIIFoldPrefixSIMD(str, pattern string) (int, bool) {
	strBytes := unsafe.Slice(unsafe.StringData(str), len(str))
	patternBytes := unsafe.Slice(unsafe.StringData(pattern), len(pattern))
	limit := min(len(strBytes), len(patternBytes))

	var vector simd.Uint8s
	width := vector.Len()
	consumed := 0
	if width <= 64 {
		upperA := simd.BroadcastInt8s('A')
		upperZ := simd.BroadcastInt8s('Z')
		caseBit := simd.BroadcastUint8s('a' - 'A')
		zero := simd.BroadcastInt8s(0)
		var maskBytes [64]int8

		for consumed+width <= limit {
			strVector := simd.LoadUint8s(strBytes[consumed:])
			patternVector := simd.LoadUint8s(patternBytes[consumed:])
			strSigned := strVector.BitsToInt8()
			patternSigned := patternVector.BitsToInt8()
			invalid := strSigned.Less(zero).Or(patternSigned.Less(zero))

			strUpper := strSigned.GreaterEqual(upperA).And(strSigned.LessEqual(upperZ))
			patternUpper := patternSigned.GreaterEqual(upperA).And(patternSigned.LessEqual(upperZ))
			strVector = strVector.Or(caseBit.Masked(strUpper))
			patternVector = patternVector.Or(caseBit.Masked(patternUpper))
			invalid = invalid.Or(strVector.NotEqual(patternVector))
			invalid.ToInt8s().Store(maskBytes[:width])

			valid := true
			for _, value := range maskBytes[:width] {
				if value != 0 {
					valid = false
					break
				}
			}
			if !valid {
				break
			}
			consumed += width
		}
	}

	for consumed < limit && strBytes[consumed] < utf8.RuneSelf && patternBytes[consumed] < utf8.RuneSelf {
		if lowerASCIIRune(rune(strBytes[consumed])) != lowerASCIIRune(rune(patternBytes[consumed])) {
			return 0, false
		}
		consumed++
	}
	return consumed, true
}
