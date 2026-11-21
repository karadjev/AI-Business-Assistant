package agentfoundation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
)

const MaxJSONBytes = 65536

// strictJSON rejects duplicate keys at any depth, null values, unknown fields,
// trailing data and non-object roots. JSON field spelling is checked by comparing
// decoded keys to the typed encoder's keys (encoding/json otherwise folds case).
func strictJSON(raw []byte, target any) error {
	if len(raw) == 0 || len(raw) > MaxJSONBytes {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := walkJSON(d, true, 0); err != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return ErrInvalid
	}
	typed, err := json.Marshal(target)
	if err != nil {
		return ErrInvalid
	}
	var a, b any
	da := json.NewDecoder(bytes.NewReader(raw))
	da.UseNumber()
	_ = da.Decode(&a)
	db := json.NewDecoder(bytes.NewReader(typed))
	db.UseNumber()
	_ = db.Decode(&b)
	if !matchingKeys(a, b) {
		return ErrInvalid
	}
	return nil
}
func matchingKeys(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			other, ok := y[k]
			if !ok || !matchingKeys(v, other) {
				return false
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !matchingKeys(x[i], y[i]) {
				return false
			}
		}
	}
	return true
}
func walkJSON(d *json.Decoder, root bool, depth int) error {
	if depth > 32 {
		return ErrInvalid
	}
	tok, err := d.Token()
	if err != nil || tok == nil {
		return ErrInvalid
	}
	delim, composite := tok.(json.Delim)
	if root && (!composite || delim != '{') {
		return ErrInvalid
	}
	if !composite {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return ErrInvalid
			}
			k, ok := key.(string)
			if !ok || seen[k] {
				return ErrInvalid
			}
			seen[k] = true
			if err = walkJSON(d, false, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := walkJSON(d, false, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = d.Token()
	return err
}
func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// ParamsHash accepts only already canonical parameters from Tool.CanonicalInput.
func ParamsHash(canonical json.RawMessage) string { return hash(string(canonical)) }

type jsonTool[I, O any] struct {
	spec    ToolSpec
	input   func(Scope, I) error
	output  func(Scope, O) error
	execute func(context.Context, Scope, I) (O, error)
}

// NewJSONTool requires concrete struct DTOs and both ownership/semantic validators.
// No arbitrary JSON map/interface root, SQL, shell or network executor is supplied.
func NewJSONTool[I, O any](spec ToolSpec, input func(Scope, I) error, output func(Scope, O) error, execute func(context.Context, Scope, I) (O, error)) (Tool, error) {
	if reflect.TypeFor[I]().Kind() != reflect.Struct || reflect.TypeFor[O]().Kind() != reflect.Struct || input == nil || output == nil || execute == nil || !valid(spec.Name) || !valid(spec.Version) || !valid(spec.Permission) || spec.CostUnits < 0 || !validEffect(spec) {
		return nil, ErrInvalid
	}
	return &jsonTool[I, O]{spec, input, output, execute}, nil
}
func (t *jsonTool[I, O]) Spec() ToolSpec { return t.spec }
func (t *jsonTool[I, O]) CanonicalInput(s Scope, raw json.RawMessage) (json.RawMessage, error) {
	var in I
	if !validScope(s) {
		return nil, ErrInvalid
	}
	if err := strictJSON(raw, &in); err != nil {
		return nil, err
	}
	if err := t.input(s, in); err != nil {
		return nil, err
	}
	b, err := json.Marshal(in)
	if err != nil || len(b) > MaxJSONBytes {
		return nil, ErrInvalid
	}
	return b, nil
}
func (t *jsonTool[I, O]) Execute(ctx context.Context, s Scope, raw json.RawMessage) (json.RawMessage, error) {
	canonical, err := t.CanonicalInput(s, raw)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var in I
	if err = json.Unmarshal(canonical, &in); err != nil {
		return nil, ErrInvalid
	}
	out, err := t.execute(ctx, s, in)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = t.output(s, out); err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, ErrInvalid
	}
	var check O
	if err = strictJSON(b, &check); err != nil {
		return nil, err
	}
	return b, nil
}

func (t *jsonTool[I, O]) ValidateOutput(s Scope, raw json.RawMessage) error {
	var out O
	if !validScope(s) {
		return ErrInvalid
	}
	if err := strictJSON(raw, &out); err != nil {
		return err
	}
	return t.output(s, out)
}

func validJSONObject(raw []byte) bool {
	if len(raw) == 0 || len(raw) > MaxJSONBytes {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if walkJSON(d, true, 0) != nil {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
