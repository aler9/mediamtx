package conf

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

var optionalGlobalValuesType = func() reflect.Type {
	var fields []reflect.StructField
	rt := reflect.TypeOf(Conf{})
	nf := rt.NumField()

	for i := 0; i < nf; i++ {
		f := rt.Field(i)
		j := f.Tag.Get("json")

		if j != "-" && j != "pathDefaults" && j != "paths" {
			if !strings.Contains(j, ",omitempty") {
				j += ",omitempty"
			}

			typ := f.Type
			if typ.Kind() != reflect.Pointer {
				typ = reflect.PointerTo(typ)
			}

			fields = append(fields, reflect.StructField{
				Name: f.Name,
				Type: typ,
				Tag:  reflect.StructTag(`json:"` + j + `"`),
			})
		}
	}

	return reflect.StructOf(fields)
}()

func newOptionalGlobalValues() interface{} {
	return reflect.New(optionalGlobalValuesType).Interface()
}

// OptionalGlobal is a Conf whose values can all be optional.
type OptionalGlobal struct {
	Values interface{}
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *OptionalGlobal) UnmarshalJSON(b []byte) error {
	p.Values = newOptionalGlobalValues()
	return jsonwrapper.Unmarshal(b, p.Values)
}

// MarshalJSON implements json.Marshaler.
func (p *OptionalGlobal) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Values)
}
