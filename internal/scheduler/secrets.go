package scheduler

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var envPlaceholder = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv replaces ${NAME} in value with the environment variable NAME,
// so devices.json (and the devices table) can hold a placeholder instead of
// the secret itself. "$${" is an escape for a literal "${". A value with no
// placeholder is returned unchanged, so literal credentials keep working.
// A placeholder whose variable is unset or empty is an error rather than a
// silent empty credential. The secret's value never appears in an error.
func expandEnv(value string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		rest := value[i:]
		if strings.HasPrefix(rest, "$${") {
			out.WriteString("${")
			i += 3
			continue
		}
		if m := envPlaceholder.FindStringSubmatch(rest); m != nil {
			v, ok := os.LookupEnv(m[1])
			if !ok || v == "" {
				return "", fmt.Errorf("environment variable %s is not set", m[1])
			}
			out.WriteString(v)
			i += len(m[0])
			continue
		}
		out.WriteByte(value[i])
		i++
	}
	return out.String(), nil
}

// resolveSecrets returns a copy of t with ${NAME} placeholders expanded in
// the credential fields. It runs when a session is built (every poll), so
// the database only ever holds the placeholder.
func resolveSecrets(t deviceTarget) (deviceTarget, error) {
	for _, f := range []struct {
		name string
		p    *string
	}{
		{"Community", &t.Community},
		{"Username", &t.Username},
		{"AuthPassphrase", &t.AuthPassphrase},
		{"PrivPassphrase", &t.PrivPassphrase},
	} {
		v, err := expandEnv(*f.p)
		if err != nil {
			return t, fmt.Errorf("device %s: %s: %w", t.Device, f.name, err)
		}
		*f.p = v
	}
	return t, nil
}
