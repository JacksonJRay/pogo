package editfile

import "crypto/rand"

func readRand(p []byte) (int, error) {
	return rand.Read(p)
}
