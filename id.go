package swim

import (
	"crypto/rand"
	"encoding/base32"
)

type id string

func randID() id {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return id(base32.StdEncoding.EncodeToString(b))
}
