//go:generate entgen
package main

import (
	"fmt"

	"github.com/rsms/ent"
	"github.com/rsms/ent/redis"
	"github.com/rsms/go-uuid"
)

type Account struct {
	ent.EntBase   `account`
	name          string `ent:",index"`
	email         string `ent:",unique"`
	emailVerified bool   `ent:"email_verified"`
	deleted       bool
	passwordHash  string `ent:"pwhash" json:"-"` // omit from json (never leak)
}

func main() {
	r := &redis.Redis{}
	if err := r.Open("127.0.0.1:6379", "", 1); err != nil {
		panic(err)
	}
	estore := redis.NewEntStorage(r)

	// create
	fmt.Printf("\n-- create --\n")
	a1 := &Account{}
	a1.SetName("bob")
	a1.SetEmail(uuid.MustGen().String() + "@bob.com")
	if err := a1.Create(estore); err != nil {
		panic(err)
	}
	fmt.Printf("created account #%d: %+v\n", a1.Id(), a1)

	// load
	fmt.Printf("\n-- load --\n")
	a2, err := LoadAccountById(estore, a1.Id())
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded account #%d: %+v\n", a1.Id(), a2)

	// save
	fmt.Printf("\n-- save --\n")
	a1.SetName("Bobby")
	a1.SetEmail(uuid.MustGen().String() + "@bob.com")
	if err := a1.Save(); err != nil {
		panic(err)
	}
	fmt.Printf("saved account #%d\n", a1.Id())

	fmt.Printf("\n-- save outdated --\n")
	a2.SetName("Bo")
	fmt.Printf("outdated.Save() => %v\n", a2.Save())
}
