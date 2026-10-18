// Package properties contains the driver that reads config from .properties files, the format of Java and Spring.
package properties

import (
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/insei/fmap/v3"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/convert"
	"github.com/insei/tinyconf/v2/drivers/internal/tree"
)

const name = "properties"

type propertiesDriver struct {
	*tree.Driver
}

// logicalLines joins the lines that end with the odd number of backslashes with the next lines,
// the leading whitespaces of the next lines are skipped. The numbers of the first lines are returned too.
func logicalLines(data string) ([]string, []int) {
	var lines []string
	var numbers []int
	var current strings.Builder
	continued := false
	for n, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		if continued {
			line = strings.TrimLeft(line, " \t\f")
		} else {
			line = strings.TrimLeft(line, " \t\f")
			if line == "" || line[0] == '#' || line[0] == '!' {
				continue
			}
			numbers = append(numbers, n+1)
		}
		backslashes := len(line) - len(strings.TrimRight(line, `\`))
		continued = backslashes%2 == 1
		if continued {
			line = line[:len(line)-1]
		}
		current.WriteString(line)
		if !continued {
			lines = append(lines, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines, numbers
}

// unescape replaces the escape sequences: \t, \n, \r, \f, \uXXXX, and \x is x.
func unescape(s string) (string, error) {
	if !strings.Contains(s, `\`) {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 'f':
			b.WriteByte('\f')
		case 'u':
			if i+5 > len(s) {
				return "", fmt.Errorf("incorrect escape sequence %q", s[i-1:])
			}
			code, err := strconv.ParseUint(s[i+1:i+5], 16, 32)
			if err != nil {
				return "", fmt.Errorf("incorrect escape sequence %q", s[i-1:i+5])
			}
			b.WriteRune(rune(code))
			i += 4
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), nil
}

// splitLine splits the logical line to the key and the value: the key ends with the first not escaped =, : or whitespace.
func splitLine(line string) (string, string) {
	end := len(line)
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if strings.IndexByte("=: \t\f", line[i]) >= 0 {
			end = i
			break
		}
	}
	key, rest := line[:end], strings.TrimLeft(line[end:], " \t\f")
	if rest != "" && (rest[0] == '=' || rest[0] == ':') {
		rest = strings.TrimLeft(rest[1:], " \t\f")
	}
	return key, rest
}

// decode decodes .properties: keys with dots are nested objects, http.port is port inside http.
func decode(data []byte) (map[string]any, error) {
	doc := make(map[string]any)
	lines, numbers := logicalLines(string(data))
	for i, line := range lines {
		rawKey, rawValue := splitLine(line)
		key, err := unescape(rawKey)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", numbers[i], err)
		}
		value, err := unescape(rawValue)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", numbers[i], err)
		}
		keys := strings.Split(key, ".")
		for _, k := range keys {
			if k == "" {
				return nil, fmt.Errorf("line %d: incorrect key %q", numbers[i], key)
			}
		}
		if err = tree.SetPath(doc, keys, value); err != nil {
			return nil, fmt.Errorf("line %d: %w", numbers[i], err)
		}
	}
	return doc, nil
}

// escape escapes the key or the value: backslashes, control characters,
// the separators and the comments in the keys, the leading spaces of the values.
func escape(s string, key bool) string {
	var b strings.Builder
	for i, r := range s {
		if r == utf8.RuneError {
			// the byte that is not UTF-8 is written as is
			if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
				b.WriteByte(s[i])
				continue
			}
		}
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\f':
			b.WriteString(`\f`)
		case key && strings.ContainsRune("=: #!", r), !key && i == 0 && strings.ContainsRune(" #!", r):
			b.WriteString(`\` + string(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// flatten writes the values of the document with the dotted keys.
func flatten(prefix string, object map[string]any, lines map[string]string) {
	for key, value := range object {
		if nested, ok := value.(map[string]any); ok {
			flatten(prefix+key+".", nested, lines)
			continue
		}
		lines[prefix+key] = value.(string)
	}
}

// GenDoc generates .properties file with all registered config fields and their current values,
// keys of the nested structs are dotted. Values of the fields with hidden:"true" tag are empty, nil values are skipped.
func (d *propertiesDriver) GenDoc(registers ...*tinyconf.Registered) string {
	values := tree.Doc(registers, name, func(v any, field fmap.Field) any {
		if valOf := reflect.ValueOf(v); !valOf.IsValid() || (valOf.Kind() == reflect.Ptr && valOf.IsNil()) {
			return nil
		}
		return convert.FieldToTagString(v, field, name)
	})
	lines := make(map[string]string)
	flatten("", values, lines)
	keys := make([]string, 0, len(lines))
	for key := range lines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var doc strings.Builder
	for _, key := range keys {
		doc.WriteString(escape(key, true) + "=" + escape(lines[key], false) + "\n")
	}
	return doc.String()
}

// Option is a properties driver option.
type Option func(*tree.Options)

// WithFiles adds additional config files. Files are merged in the order of addition,
// values from the next files overrides values from the previous.
func WithFiles(files ...string) Option {
	return func(o *tree.Options) {
		o.AddFiles(files...)
	}
}

// WithFS adds config files from the fsys, embed.FS for example.
func WithFS(fsys fs.FS, files ...string) Option {
	return func(o *tree.Options) {
		o.AddFS(fsys, files...)
	}
}

// WithData adds in-memory config content, name is used in error messages.
func WithData(name string, data []byte) Option {
	return func(o *tree.Options) {
		o.AddData(name, data)
	}
}

// WithSource adds the config content from another source, from a remote storage for example:
// consul.Document reads the content from Consul KV. Sources are merged with the files in the order of addition.
func WithSource(source tree.Source) Option {
	return func(o *tree.Options) {
		o.AddSource(source)
	}
}

// WithOptional allows config files to be absent, by default absent file is an error.
func WithOptional() Option {
	return func(o *tree.Options) {
		o.Optional = true
	}
}

// WithEnvExpansion replaces ${VAR} and ${VAR:-default} in the config content with the values of the environment variables.
// Default is used when variable is not set or empty, $$ is replaced with $.
func WithEnvExpansion() Option {
	return func(o *tree.Options) {
		o.ExpandEnv = true
	}
}

// WithWatchInterval sets the interval of polling the config files for changes while there are subscriptions, 5 seconds by default.
// The files are watched by the events of the file system too, polling detects the changes if the events are lost.
func WithWatchInterval(interval time.Duration) Option {
	return func(o *tree.Options) {
		o.WatchInterval = interval
	}
}

// New creates new properties driver. Keys are taken from the properties struct tags, the tags of the parents are
// the parts of the dotted keys: properties:"port" inside properties:"http" is read from the http.port key.
// File can be empty if config sources are added by options.
func New(file string, opts ...Option) (tinyconf.Driver, error) {
	o := &tree.Options{}
	o.AddFiles(file)
	for _, opt := range opts {
		opt(o)
	}
	return &propertiesDriver{Driver: o.NewDriver(name, decode)}, nil
}
