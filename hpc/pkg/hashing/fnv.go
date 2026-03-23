package hashing

import "hash/fnv"

// HashToRange accepts a string and number to hash
// a string into the range of [0, N].
func HashToRange(s string, N uint64) int {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int(h.Sum64() % N)
}
