// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"reflect"
	"testing"

	"github.com/omec-project/amf/factory"
)

// TestTheKeepaliveSettingsReachTheInterceptionSubsystem: every keepalive key an operator can set
// in the li block must reach Config. One dropped in the mapping is accepted by the strict decode
// and then silently ignored — an operator who agreed "no keepalives" with the agency gets the
// default procedure deactivating tasking anyway.
func TestTheKeepaliveSettingsReachTheInterceptionSubsystem(t *testing.T) {
	cfg := ConfigFromFactory(&factory.Li{
		KeepaliveTimeout:       "2h",
		KeepaliveTimeP1:        "2m",
		KeepaliveTimeP3:        "3h",
		KeepaliveRetainTasking: true,
		KeepaliveDisabled:      true,
	}, nil)

	if cfg.KeepaliveTimeout != "2h" || cfg.KeepaliveTimeP1 != "2m" || cfg.KeepaliveTimeP3 != "3h" {
		t.Errorf("timers not carried: %q %q %q", cfg.KeepaliveTimeout, cfg.KeepaliveTimeP1, cfg.KeepaliveTimeP3)
	}
	if !cfg.KeepaliveRetainTasking {
		t.Error("keepaliveRetainTasking was dropped")
	}
	if !cfg.KeepaliveDisabled {
		t.Error("keepaliveDisabled was dropped")
	}
}

// TestEveryLiKeyReachesTheInterceptionSubsystem: every field of the li block, not only the
// keepalive ones, must arrive in Config under the same name. A key the mapping drops is accepted
// by the strict decode and then silently ignored, so an operator who agreed a bulk-operation
// setting or a delivery timer with the agency gets the default instead.
func TestEveryLiKeyReachesTheInterceptionSubsystem(t *testing.T) {
	var li factory.Li
	fillEvery(t, reflect.ValueOf(&li).Elem())

	cfg := ConfigFromFactory(&li, nil)

	for _, b := range sameByName(reflect.ValueOf(li), reflect.ValueOf(cfg), "") {
		t.Error(b)
	}
}

// fillEvery sets every exported field of v to a distinct non-zero value, so a mapping that
// drops one is caught by comparing names rather than by remembering which keys exist.
func fillEvery(t *testing.T, v reflect.Value) {
	t.Helper()

	for i := range v.NumField() {
		f, sf := v.Field(i), v.Type().Field(i)
		if !sf.IsExported() {
			continue
		}

		switch f.Kind() {
		case reflect.String:
			f.SetString("value-of-" + sf.Name)
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int64, reflect.Int32, reflect.Uint32, reflect.Uint64:
			f.Set(reflect.ValueOf(7).Convert(f.Type()))
		case reflect.Pointer:
			f.Set(reflect.New(f.Type().Elem()))
			if f.Elem().Kind() == reflect.Struct {
				fillEvery(t, f.Elem())
			} else {
				fillScalar(t, f.Elem(), sf.Name)
			}
		case reflect.Slice:
			f.Set(reflect.MakeSlice(f.Type(), 1, 1))
			if f.Index(0).Kind() == reflect.Struct {
				fillEvery(t, f.Index(0))
			} else {
				fillScalar(t, f.Index(0), sf.Name)
			}
		case reflect.Struct:
			fillEvery(t, f)
		default:
			t.Fatalf("factory field %s has kind %s, which this test does not fill: extend it", sf.Name, f.Kind())
		}
	}
}

func fillScalar(t *testing.T, v reflect.Value, name string) {
	t.Helper()

	switch v.Kind() {
	case reflect.String:
		v.SetString("value-of-" + name)
	case reflect.Bool:
		v.SetBool(true)
	default:
		t.Fatalf("factory field %s holds kind %s, which this test does not fill: extend it", name, v.Kind())
	}
}

// sameByName reports where dst does not carry src's value under the same field name.
func sameByName(src, dst reflect.Value, path string) []string {
	var bad []string

	for i := range src.NumField() {
		sf := src.Type().Field(i)
		if !sf.IsExported() {
			continue
		}

		name := path + sf.Name
		df := dst.FieldByName(sf.Name)
		if !df.IsValid() {
			bad = append(bad, name+": no field of that name in the interception config")
			continue
		}

		s := src.Field(i)
		switch {
		case s.Kind() == reflect.Slice && s.Len() > 0 && s.Index(0).Kind() == reflect.Struct:
			if df.Len() != s.Len() {
				bad = append(bad, name+": entries not carried")
				continue
			}
			bad = append(bad, sameByName(s.Index(0), df.Index(0), name+"[0].")...)
		case s.Type() == df.Type():
			if !reflect.DeepEqual(s.Interface(), df.Interface()) {
				bad = append(bad, name+": value not carried")
			}
		default:
			bad = append(bad, name+": carried as a different type ("+df.Type().String()+")")
		}
	}

	return bad
}
