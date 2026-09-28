// Command crmctl performs administration tasks: migrations, stores, user accounts and demo data.
//
// Passwords are read from CRM_PASSWORD or, when unset, from the first line of standard input,
// so they never appear in shell history or process listings.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/auth"
	"vodafone/store/internal/crm"
	"vodafone/store/internal/demo"
	"vodafone/store/internal/postgres"
)

const usage = `usage: crmctl <command> [flags]

commands:
  migrate                                         apply database migrations
  create-store -name NAME [-timezone TZ]          create a store and print its ID
  list-stores                                     list stores
  create-user -store ID -email E -name N -role R  create an account (role: employee or manager)
  reset-password -email E                         set a new password and revoke sessions
  deactivate-user -email E                        disable an account and revoke sessions
  seed-demo                                       create the fictional demo store (DEMO_PASSWORD or generated)

DATABASE_URL must be set.`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		var e *apperr.Error
		if errors.As(err, &e) && e.Fields != nil {
			fmt.Fprintf(os.Stderr, "error: %s %v\n", e.Code, e.Fields)
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := postgres.Open(ctx, url, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	authSvc := auth.NewService(db, auth.DefaultConfig())
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)

	switch cmd {
	case "migrate":
		fmt.Println("migrations applied")
		return nil
	case "create-store":
		name := fs.String("name", "", "store name")
		tz := fs.String("timezone", "Europe/Bucharest", "IANA time zone")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if strings.TrimSpace(*name) == "" {
			return errors.New("-name is required")
		}
		if _, err := time.LoadLocation(*tz); err != nil {
			return fmt.Errorf("unknown time zone %q", *tz)
		}
		s := crm.RetailStore{ID: crm.NewID(), Name: strings.TrimSpace(*name), Timezone: *tz}
		if err := db.InsertStore(ctx, s); err != nil {
			return err
		}
		fmt.Println(s.ID)
		return nil
	case "list-stores":
		stores, err := db.Stores(ctx)
		if err != nil {
			return err
		}
		for _, s := range stores {
			fmt.Printf("%s\t%s\t%s\n", s.ID, s.Timezone, s.Name)
		}
		return nil
	case "create-user":
		store := fs.String("store", "", "store ID")
		email := fs.String("email", "", "email")
		name := fs.String("name", "", "display name")
		role := fs.String("role", crm.RoleEmployee, "employee or manager")
		if err := fs.Parse(args); err != nil {
			return err
		}
		password, err := readPassword()
		if err != nil {
			return err
		}
		u, err := authSvc.CreateUser(ctx, auth.NewUser{StoreID: *store, Email: *email, Name: *name, Role: *role, Password: password})
		if err != nil {
			return err
		}
		fmt.Println(u.ID)
		return nil
	case "reset-password":
		email := fs.String("email", "", "email")
		if err := fs.Parse(args); err != nil {
			return err
		}
		password, err := readPassword()
		if err != nil {
			return err
		}
		if err := authSvc.ResetPassword(ctx, *email, password); err != nil {
			return err
		}
		fmt.Println("password updated; existing sessions revoked")
		return nil
	case "deactivate-user":
		email := fs.String("email", "", "email")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if err := db.SetUserActive(ctx, auth.NormalizeEmail(*email), false); err != nil {
			return err
		}
		fmt.Println("account disabled; sessions revoked")
		return nil
	case "seed-demo":
		password := os.Getenv("DEMO_PASSWORD")
		generated := password == ""
		if generated {
			b := make([]byte, 12)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			password = base64.RawURLEncoding.EncodeToString(b)
		}
		err := demo.Seed(ctx, db, authSvc, password, time.Now())
		if errors.Is(err, demo.ErrAlreadySeeded) {
			fmt.Println("demo store already exists; use reset-password to change a demo password")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Println("demo store created with fictional data. Accounts:")
		for _, a := range demo.Accounts {
			fmt.Printf("  %-30s %s\n", a.Email, a.Role)
		}
		if generated {
			fmt.Printf("password for all demo accounts: %s\n", password)
		} else {
			fmt.Println("password: the value of DEMO_PASSWORD")
		}
		return nil
	}
	fmt.Fprintln(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

func readPassword() (string, error) {
	if p := os.Getenv("CRM_PASSWORD"); p != "" {
		return p, nil
	}
	fmt.Fprint(os.Stderr, "password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password provided (set CRM_PASSWORD or pipe it on stdin)")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
