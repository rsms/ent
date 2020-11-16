//go:generate entgen
package main

import (
	"fmt"
	"github.com/rsms/ent"
	"github.com/rsms/ent/mem"
)

type AccountKind int

const (
	AccountRestricted = AccountKind(iota)
	AccountMember
	AccountAdmin
)

type Account struct {
	ent.EntBase `account` // type name, used to storage these kinds of ents
	name        string
	displayName string      `ent:"alias"`   // use a different field name for storage
	email       string      `ent:",unique"` // maintain a unique index for this field
	kind        AccountKind `ent:",index"`  // maintain a (non-unique) index for this field
}

func main() {
	a := &Account{
		name:  "Jane",
		email: "jane@example.com",
		kind:  AccountMember,
	}
	println(a.String())

	estore := mem.NewEntStorage()
	if err := a.Create(estore); err != nil {
		panic(err)
	}
	println(a.String())

	a, _ = LoadAccountById(estore, 1)
	fmt.Printf("account #1: %+v\n", a)

	b, _ := LoadAccountByEmail(estore, "jane@example.com")
	fmt.Printf("account with email jane@example.com: %v\n", b)

	_, err := LoadAccountByEmail(estore, "does@not.exist")
	fmt.Printf("error from lookup of non-existing email: %v\n", err)

	id, _ := FindAccountByEmail(estore, "jane@example.com")
	fmt.Printf("id of account with email jane@example.com: %v\n", id)

	(&Account{email: "robin@foo.com", kind: AccountMember}).Create(estore)
	(&Account{email: "thor@xy.z", kind: AccountAdmin}).Create(estore)
	(&Account{email: "alice@es.gr", kind: AccountRestricted}).Create(estore)

	accounts, _ := LoadAccountByKind(estore, AccountMember, 0)
	fmt.Printf("'member' accounts: %+v\n", accounts)

	accounts, _ = LoadAccountByKind(estore, AccountAdmin, 0)
	fmt.Printf("'admin' accounts: %+v\n", accounts)

	err = (&Account{email: "jane@example.com"}).Create(estore)
	fmt.Printf("error (duplicate email): %v\n", err)

	a, _ = LoadAccountByEmail(estore, "robin@foo.com")
	a.SetEmail("jane@example.com")
	fmt.Printf("error (duplicate email): %v\n", a.Save())

	a, _ = LoadAccountById(estore, 1)
	a.SetEmail("jane.smith@foo.z")
	a.Save()

	a, _ = LoadAccountByEmail(estore, "robin@foo.com")
	a.SetEmail("jane@example.com")
	fmt.Printf("no error: %v\n", a.Save())
}
