package fireditto

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go/v4/auth"
)

//go:generate stringer -type=Mode -trimprefix=Mode
type Mode uint8

const (
	ModeUser Mode = iota
	ModeMem
)

func (m *Mode) Parse(mode string) error {
	switch strings.ToLower(mode) {
	case "user":
		*m = ModeUser
	case "mem":
		*m = ModeMem
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
	return nil
}

//go:generate stringer -type=Op -trimprefix=Op
type Op uint8

const (
	OpGET Op = iota
	OpEmbed
	OpDeleteColumn
)

func (o *Op) Parse(operation string) error {
	switch strings.ToLower(operation) {
	case "get":
		*o = OpGET
	case "embed":
		*o = OpEmbed
	case "delcol", "delete-column":
		*o = OpDeleteColumn
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}
	return nil
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
		Embed        CommandEmbed
		DeleteColumn string
		AllUsers     bool
	}
	fs   *firestore.Client
	auth *auth.Client
}

type CommandEmbed struct {
	ContentField string
	EmbedField   string
	ModelVersion int
	Start        timeVar
}

func (e *CommandEmbed) Validate() error {
	if e.ContentField == "" {
		return errors.New("content field must be provided")
	}
	if e.EmbedField == "" {
		return errors.New("embed field must be provided")
	}
	if e.ModelVersion < 4 || e.ModelVersion > 5 {
		return fmt.Errorf("invalid model version: %d", e.ModelVersion)
	}
	return nil
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
	if err := f.Mode.Parse(args[0]); err != nil {
		return fmt.Errorf("invalid mode: %s", err)
	}
	if err := f.Operation.Parse(args[1]); err != nil {
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
		firestoreFlags.BoolVar(&f.Mem.AllUsers, "all-users", false, "all users")
		switch f.Operation {
		case OpEmbed:
			firestoreFlags.StringVar(&f.Mem.Embed.ContentField, "content-field", "prompt", "content field")
			firestoreFlags.StringVar(&f.Mem.Embed.EmbedField, "embed-field", "embedding_prompt", "embed field")
			firestoreFlags.IntVar(&f.Mem.Embed.ModelVersion, "model-version", 5, "model version")
			firestoreFlags.Var(&f.Mem.Embed.Start, "start", "start time")
		case OpDeleteColumn:
			firestoreFlags.StringVar(&f.Mem.DeleteColumn, "col", "embedding", "delete column")
		}
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
		switch f.Operation {
		case OpEmbed:
			if f.Mem.Embed.ContentField == "" {
				return errors.New("content field must be provided")
			}
			if f.Mem.Embed.EmbedField == "" {
				return errors.New("embed field must be provided")
			}
		case OpDeleteColumn:
			if f.Mem.DeleteColumn == "" {
				return errors.New("column to delete must be provided")
			}
		}
	}
	return nil
}

func (f *Command) Handle(ctx context.Context) error {
	switch f.Mode {
	case ModeUser:
		return f.handleUser(ctx)
	case ModeMem:
		return f.handleMem(ctx)
	default:
		return fmt.Errorf("unknown mode: %s", f.Mode)
	}
}

func (f *Command) handleUser(ctx context.Context) error {
	switch f.Operation {
	case OpGET:
		return f.printUser(ctx)
	default:
		return fmt.Errorf("unknown operation: %s", f.Operation)
	}
}

func (f *Command) handleMem(ctx context.Context) error {
	switch f.Operation {
	case OpEmbed:
		return f.embedMem(ctx)
	case OpDeleteColumn:
		return f.deleteColumn(ctx)
	default:
		return fmt.Errorf("unknown operation: %s", f.Operation)
	}
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

func (t timeVar) Time() time.Time {
	return time.Time(t)
}
