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

	// create a second account with the same email
	fmt.Printf("\n-- create with duplicate email --\n")
	a2 := &Account{}
	a2.SetEmail(a1.Email())
	err := a2.Create(estore)
	fmt.Printf("create w same email [should fail] => error: %v\n", err)

	// load
	fmt.Printf("\n-- load --\n")
	a3, err := LoadAccountById(estore, a1.Id())
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded account #%d: %+v\n", a1.Id(), a3)

	// save
	fmt.Printf("\n-- save --\n")
	a1.SetName("Bobby")
	a1.SetEmail(uuid.MustGen().String() + "@bob.com")
	if err := a1.Save(); err != nil {
		panic(err)
	}
	fmt.Printf("saved account #%d\n", a1.Id())

	// save outdated should fail
	fmt.Printf("\n-- save outdated --\n")
	a3.SetName("Bo")
	fmt.Printf("outdated.Save() [should fail] => error: %v\n", a3.Save())

	// load by unique index
	fmt.Printf("\n-- load by unique index 'email' --\n")
	a4, err := LoadAccountByEmail(estore, a1.Email())
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded account by email %q: %+v\n", a1.Email(), a4)

	// create another account with the same name
	fmt.Printf("\n-- create --\n")
	a5 := &Account{}
	a5.SetName(a1.Name())
	if err := a5.Create(estore); err != nil {
		panic(err)
	}

	// find by non-unique index
	fmt.Printf("\n-- find by non-unique index 'name' --\n")
	ids, err := FindAccountByName(estore, a1.Name(), 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded account IDs with name %q: %+v\n", a1.Name(), ids)

	// delete an account
	fmt.Printf("\n-- delete --\n")
	if err := a1.PermanentlyDelete(); err != nil {
		panic(err)
	}

	fmt.Printf("\n-- load non-existing ent --\n")
	_, err = LoadAccountById(estore, a1.Id())
	fmt.Printf("LoadAccountById(%d) [should fail] => error: %v\n", a1.Id(), err)

	estore.Close()
	r.Close()
}
