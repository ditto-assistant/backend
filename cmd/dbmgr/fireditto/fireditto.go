package fireditto

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

//go:generate stringer -type=Mode -trimprefix=Mode
type Mode uint8

const (
	ModeUser Mode = iota
	ModeMem
)

func ParseMode(mode string) (Mode, error) {
	switch strings.ToLower(mode) {
	case "user":
		return ModeUser, nil
	case "mem":
		return ModeMem, nil
	default:
		return 0, fmt.Errorf("unknown mode: %s", mode)
	}
}

//go:generate stringer -type=Op -trimprefix=Op
type Op uint8

const (
	OpGET Op = iota
	OpEmbed
)

func ParseCrudOperation(operation string) (Op, error) {
	switch strings.ToLower(operation) {
	case "get":
		return OpGET, nil
	case "embed":
		return OpEmbed, nil
	default:
		return 0, fmt.Errorf("unknown operation: %s", operation)
	}
}

type Command struct {
	Mode       Mode
	Operation  Op
	Email, UID string
	User       struct {
		Offset, Limit int
		order         string
	}
	Mem struct {
		Embed CommandEmbed
	}
}

type CommandEmbed struct {
	ContentField string
	EmbedField   string
	ModelVersion int
	Start        time.Time
}

func (f *Command) Order() firestore.Direction {
	switch strings.ToLower(f.User.order) {
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

	switch f.Mode {
	case ModeUser:
		firestoreFlags.IntVar(&f.User.Offset, "offset", 0, "offset")
		firestoreFlags.IntVar(&f.User.Limit, "limit", 100, "limit")
		firestoreFlags.StringVar(&f.User.order, "order", "asc", "order")
		firestoreFlags.Parse(args[2:])
		if err := f.Validate(); err != nil {
			return fmt.Errorf("invalid user flags: %s", err)
		}
	case ModeMem:
		firestoreFlags.StringVar(&f.Mem.Embed.ContentField, "content-field", "", "content field")
		firestoreFlags.StringVar(&f.Mem.Embed.EmbedField, "embed-field", "", "embed field")
		firestoreFlags.IntVar(&f.Mem.Embed.ModelVersion, "model-version", 1, "model version")
		firestoreFlags.Var((*timeVar)(&f.Mem.Embed.Start), "start", "start time (optional)")
		firestoreFlags.Parse(args[2:])
		if err := f.Validate(); err != nil {
			return fmt.Errorf("invalid mem flags: %s", err)
		}
	}
	return nil
}

func (f *Command) Validate() error {
	switch f.Mode {
	case ModeUser:
		if f.Email == "" && f.UID == "" {
			return errors.New("either email or uid must be provided")
		}
	case ModeMem:
		if f.Mem.Embed.ContentField == "" {
			return errors.New("content field must be provided")
		}
		if f.Mem.Embed.EmbedField == "" {
			return errors.New("embed field must be provided")
		}
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
	case OpGET:
		return f.PrintUser(ctx)
	default:
		return fmt.Errorf("unknown operation: %s", f.Operation)
	}
}

// multiFlag implements flag.Value interface for repeated string flags
type multiFlag []string

func (f *multiFlag) String() string {
	return strings.Join(*f, ", ")
}

func (f *multiFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

// intMultiFlag implements flag.Value interface for repeated int flags
type intMultiFlag []int

func (f *intMultiFlag) String() string {
	var s []string
	for _, i := range *f {
		s = append(s, fmt.Sprint(i))
	}
	return strings.Join(s, ", ")
}

func (f *intMultiFlag) Set(value string) error {
	i, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	*f = append(*f, i)
	return nil
}

// timeVar implements flag.Value interface for a single time.Time value
type timeVar time.Time

func (t *timeVar) String() string {
	return time.Time(*t).Format(time.RFC3339)
}

func (t *timeVar) Set(value string) error {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return err
	}
	*t = timeVar(parsed)
	return nil
}
