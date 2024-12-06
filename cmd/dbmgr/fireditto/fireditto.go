package fireditto

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"cloud.google.com/go/firestore"
)

type Mode string

const (
	ModeUser Mode = "user"
)

func ParseMode(mode string) (Mode, error) {
	switch mode {
	case "user":
		return ModeUser, nil
	default:
		return "", fmt.Errorf("unknown mode: %s", mode)
	}
}

type CrudOperation string

const (
	SubtypeUserGet CrudOperation = "get"
)

func ParseCrudOperation(operation string) (CrudOperation, error) {
	switch operation {
	case "get":
		return SubtypeUserGet, nil
	default:
		return "", fmt.Errorf("unknown operation: %s", operation)
	}
}

type Command struct {
	Mode      Mode
	Operation CrudOperation
	Email     string
	UID       string
	Offset    int
	Limit     int
	order     string
}

func (f *Command) Order() firestore.Direction {
	switch strings.ToLower(f.order) {
	case "asc":
		return firestore.Asc
	case "desc":
		return firestore.Desc
	default:
		return firestore.Asc
	}
}

func (f *Command) Parse(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("no operation provided")
	}
	mode, err := ParseMode(args[0])
	if err != nil {
		return fmt.Errorf("invalid mode: %s", err)
	}
	f.Mode = mode
	f.Operation, err = ParseCrudOperation(args[1])
	if err != nil {
		return fmt.Errorf("invalid operation: %s", err)
	}
	firestoreFlags := flag.NewFlagSet("firestore", flag.ExitOnError)
	firestoreFlags.StringVar(&f.UID, "uid", "", "user ID")
	firestoreFlags.StringVar(&f.Email, "email", "", "user email")
	firestoreFlags.IntVar(&f.Offset, "offset", 0, "offset")
	firestoreFlags.IntVar(&f.Limit, "limit", 100, "limit")
	firestoreFlags.StringVar(&f.order, "order", "asc", "order")
	firestoreFlags.Parse(args[2:])
	if err := f.Validate(); err != nil {
		return fmt.Errorf("invalid firebase flags: %s", err)
	}
	return nil
}

func (f *Command) Validate() error {
	if f.Email == "" && f.UID == "" {
		return errors.New("either email or uid must be provided")
	}
	return nil
}

func (f *Command) Handle(ctx context.Context) error {
	switch f.Mode {
	case ModeUser:
		return f.HandleUser(ctx)
	default:
		return fmt.Errorf("unknown mode: %s", f.Mode)
	}
}

func (f *Command) HandleUser(ctx context.Context) error {
	switch f.Operation {
	case SubtypeUserGet:
		return f.PrintUser(ctx)
	default:
		return fmt.Errorf("unknown operation: %s", f.Operation)
	}
}
