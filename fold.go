//go:build !go1.27 || !goexperiment.simd

package redglob

func matchASCIIFoldPrefix(str, pattern string) (int, bool) {
	return matchASCIIFoldPrefixScalar(str, pattern)
}
