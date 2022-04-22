package main

import (
	"math/rand"
)

func (c *config) F_insertData() (ret int64, err error) {
	if c.InsertData.Version == "4" {
		return c.F_insertData4()
	}
	return c.F_insertData5()
}

func RandStringRunes(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
