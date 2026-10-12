// Package flag contains the driver that reads config from the command line flags.
package flag

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/convert"
)

type flagDriver struct {
	name string
	args []string
}

func (d flagDriver) getFlagName(field fmap.Field) string {
	return strings.Split(field.GetTag().Get(d.name), ",")[0]
}

// find returns values of all occurrences of the flag in the command line arguments.
func (d flagDriver) find(flagName string, isBool bool) ([]string, error) {
	var values []string
	for i := 0; i < len(d.args); i++ {
		arg := d.args[i]
		if arg == "--" {
			// all arguments after -- are not flags
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			continue
		}
		arg = strings.TrimPrefix(arg[1:], "-")
		argName, argValue, hasValue := arg, "", false
		if idx := strings.Index(arg, "="); idx >= 0 {
			argName, argValue, hasValue = arg[:idx], arg[idx+1:], true
		}
		if argName != flagName {
			continue
		}
		switch {
		case hasValue:
		case isBool:
			argValue = "true"
		case i+1 < len(d.args):
			i++
			argValue = d.args[i]
		default:
			return nil, fmt.Errorf("flag needs an argument: -%s", flagName)
		}
		values = append(values, argValue)
	}
	return values, nil
}

func (d flagDriver) GetValue(field fmap.Field) (*tinyconf.Value, error) {
	flagName := d.getFlagName(field)
	if flagName == "" || flagName == "-" {
		return nil, fmt.Errorf("%w: %s tag is not set for %s config field", tinyconf.ErrIncorrectTagSettings, d.name, field.GetStructPath())
	}
	typeOf := field.GetType()
	for typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}
	values, err := d.find(flagName, typeOf.Kind() == reflect.Bool)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: flag -%s is not provided for %s config field", tinyconf.ErrValueNotFound, flagName, field.GetStructPath())
	}
	// last flag wins, but slices and arrays can be filled with repeated flags
	valueStr := values[len(values)-1]
	if len(values) > 1 && (typeOf.Kind() == reflect.Slice || typeOf.Kind() == reflect.Array) {
		separator := field.GetTag().Get(convert.SeparatorTag)
		if separator == "" {
			separator = convert.DefaultSeparator
		}
		valueStr = strings.Join(values, separator)
	}
	value, err := convert.StringToField(valueStr, field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse value of the flag -%s for %s config field: %w", flagName, field.GetStructPath(), err)
	}
	return &tinyconf.Value{Source: "-" + flagName, Value: value}, nil
}

func (d flagDriver) GetName() string {
	return d.name
}

// GenDoc generates usage of all flags of the registered configs with their current values as defaults.
// Values of the fields with hidden:"true" tag are not shown.
func (d flagDriver) GenDoc(registers ...*tinyconf.Registered) string {
	var doc strings.Builder
	documented := make(map[string]struct{})
	for _, register := range registers {
		for _, path := range register.Storage.GetAllPaths() {
			fld := register.Storage.MustFind(path)
			flagName := d.getFlagName(fld)
			if flagName == "" || flagName == "-" || !convert.IsValueField(register.Storage, fld) {
				continue
			}
			if _, ok := documented[flagName]; ok {
				continue
			}
			documented[flagName] = struct{}{}

			fmt.Fprintf(&doc, "  -%s\n    \t%s", flagName, fld.GetTag().Get("doc"))
			if fld.GetTag().Get("hidden") != "true" {
				if value := convert.FieldToString(convert.GetFieldValue(fld, register.Config), fld); value != "" {
					fmt.Fprintf(&doc, " (default %s)", value)
				}
			}
			doc.WriteString("\n")
		}
	}
	return doc.String()
}

type options struct {
	args []string
}

// Option is a flag driver option.
type Option func(*options)

// WithArgs sets command line arguments without the program name, by default os.Args[1:] is used.
func WithArgs(args []string) Option {
	return func(o *options) {
		o.args = args
	}
}

// New creates new flag driver. Names of the flags are taken from the flag struct tags.
//
// Supported forms of the flags: -name=value, --name=value, -name value, --name value,
// -name and --name for bool fields. Slices and arrays can be filled with repeated flags.
// All arguments after -- are ignored. The driver does not know all flags of the application,
// so unknown flags are ignored.
func New(opts ...Option) (tinyconf.Driver, error) {
	o := &options{args: os.Args[1:]}
	for _, opt := range opts {
		opt(o)
	}
	return flagDriver{name: "flag", args: o.args}, nil
}
