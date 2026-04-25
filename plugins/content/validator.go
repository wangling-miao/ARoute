package content

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	colorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	slugRegex  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9\-]*[a-z0-9])?$`)
)

// FieldValidator validates content data against content type field definitions.
type FieldValidator struct {
	store *Store
}

// NewFieldValidator creates a FieldValidator with a store reference for relation validation.
func NewFieldValidator(store *Store) *FieldValidator {
	return &FieldValidator{store: store}
}

// Validate checks all fields in data against the content type definition.
// Returns a *interfaces.ValidationErrors containing ALL validation failures (not just the first).
func (v *FieldValidator) Validate(ctx context.Context, ct *interfaces.ContentType, data map[string]interface{}) error {
	verrs := interfaces.NewValidationErrors()

	fieldMap := make(map[string]*interfaces.Field, len(ct.Fields))
	for i := range ct.Fields {
		fieldMap[ct.Fields[i].Name] = &ct.Fields[i]
	}

	for i := range ct.Fields {
		field := &ct.Fields[i]
		val, exists := data[field.Name]

		if !exists || isEmpty(val) {
			if field.Required {
				verrs.Add(field.Name, "field is required", "required")
			}
			continue
		}

		v.validateField(ctx, ct.Name, field, val, verrs)
	}

	if verrs.HasErrors() {
		return verrs
	}
	return nil
}

func isEmpty(val interface{}) bool {
	if val == nil {
		return true
	}
	switch v := val.(type) {
	case string:
		return v == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	default:
		return false
	}
}

func (v *FieldValidator) validateField(ctx context.Context, contentType string, field *interfaces.Field, val interface{}, verrs *interfaces.ValidationErrors) {
	switch field.Type {
	case "text", "markdown", "richtext":
		v.validateText(field, val, verrs)
	case "number":
		v.validateNumber(field, val, verrs)
	case "boolean":
		v.validateBoolean(val, field.Name, verrs)
	case "date":
		v.validateDate(val, field.Name, verrs)
	case "datetime":
		v.validateDatetime(val, field.Name, verrs)
	case "email":
		v.validateEmail(val, field.Name, verrs)
	case "url":
		v.validateURL(val, field.Name, verrs)
	case "slug":
		v.validateSlug(val, field.Name, verrs)
	case "enum":
		v.validateEnum(field, val, verrs)
	case "color":
		v.validateColor(val, field.Name, verrs)
	case "json":
		v.validateJSON(val, field.Name, verrs)
	case "media":
		v.validateMedia(val, field.Name, verrs)
	case "relation":
		v.validateRelation(ctx, contentType, field, val, field.Name, verrs)
	default:
		verrs.Add(field.Name, fmt.Sprintf("unsupported field type: %s", field.Type), "unsupported_type")
	}
}

func (v *FieldValidator) validateText(field *interfaces.Field, val interface{}, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(field.Name, "must be a string", "type_mismatch")
		return
	}

	rules := field.ValidationRules
	if rules == nil {
		return
	}

	if minLen, ok := rules["minLength"]; ok {
		if min, err := toInt(minLen); err == nil && len(s) < min {
			verrs.Add(field.Name, fmt.Sprintf("must be at least %d characters", min), "min_length")
		}
	}

	if maxLen, ok := rules["maxLength"]; ok {
		if max, err := toInt(maxLen); err == nil && len(s) > max {
			verrs.Add(field.Name, fmt.Sprintf("must be at most %d characters", max), "max_length")
		}
	}

	if pattern, ok := rules["pattern"]; ok {
		pat, ok := pattern.(string)
		if ok && pat != "" {
			re, err := regexp.Compile(pat)
			if err == nil && !re.MatchString(s) {
				verrs.Add(field.Name, fmt.Sprintf("must match pattern %s", pat), "pattern")
			}
		}
	}
}

func (v *FieldValidator) validateNumber(field *interfaces.Field, val interface{}, verrs *interfaces.ValidationErrors) {
	num, ok := toFloat64(val)
	if !ok {
		verrs.Add(field.Name, "must be a number", "type_mismatch")
		return
	}

	rules := field.ValidationRules
	if rules == nil {
		return
	}

	if minVal, ok := rules["min"]; ok {
		if min, err := toFloat64E(minVal); err == nil && num < min {
			verrs.Add(field.Name, fmt.Sprintf("must be at least %v", min), "min")
		}
	}

	if maxVal, ok := rules["max"]; ok {
		if max, err := toFloat64E(maxVal); err == nil && num > max {
			verrs.Add(field.Name, fmt.Sprintf("must be at most %v", max), "max")
		}
	}
}

func (v *FieldValidator) validateBoolean(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	switch val.(type) {
	case bool:
		return
	case string:
		s := strings.ToLower(val.(string))
		if s == "true" || s == "false" || s == "1" || s == "0" {
			return
		}
	case float64, int, int64:
		return
	default:
	}
	verrs.Add(name, "must be a boolean", "type_mismatch")
}

func (v *FieldValidator) validateDate(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(name, "must be a string", "type_mismatch")
		return
	}
	if !dateRegex.MatchString(s) {
		verrs.Add(name, "must be a valid ISO 8601 date (YYYY-MM-DD)", "invalid_date")
	}
}

func (v *FieldValidator) validateDatetime(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(name, "must be a string", "type_mismatch")
		return
	}
	if !strings.Contains(s, "T") && !strings.Contains(s, " ") {
		verrs.Add(name, "must be a valid ISO 8601 datetime", "invalid_datetime")
		return
	}
}

func (v *FieldValidator) validateEmail(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(name, "must be a string", "type_mismatch")
		return
	}
	if !emailRegex.MatchString(s) {
		verrs.Add(name, "must be a valid email address", "invalid_email")
	}
}

func (v *FieldValidator) validateURL(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(name, "must be a string", "type_mismatch")
		return
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		verrs.Add(name, "must be a valid URL", "invalid_url")
	}
}

func (v *FieldValidator) validateSlug(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(name, "must be a string", "type_mismatch")
		return
	}
	if !slugRegex.MatchString(s) {
		verrs.Add(name, "must be URL-safe (lowercase, hyphens, alphanumeric)", "invalid_slug")
	}
}

func (v *FieldValidator) validateEnum(field *interfaces.Field, val interface{}, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(field.Name, "must be a string", "type_mismatch")
		return
	}

	rules := field.ValidationRules
	if rules == nil {
		return
	}

	enumVals, ok := rules["enum"]
	if !ok {
		return
	}

	var allowed []string
	switch ev := enumVals.(type) {
	case []string:
		allowed = ev
	case []interface{}:
		for _, v := range ev {
			if vs, ok := v.(string); ok {
				allowed = append(allowed, vs)
			}
		}
	}

	for _, a := range allowed {
		if s == a {
			return
		}
	}
	verrs.Add(field.Name, fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")), "invalid_enum")
}

func (v *FieldValidator) validateColor(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	s, ok := val.(string)
	if !ok {
		verrs.Add(name, "must be a string", "type_mismatch")
		return
	}
	if !colorRegex.MatchString(s) {
		verrs.Add(name, "must be a valid hex color (e.g. #FF0000)", "invalid_color")
	}
}

func (v *FieldValidator) validateJSON(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	switch val.(type) {
	case map[string]interface{}, []interface{}, string:
		if s, ok := val.(string); ok {
			if !json.Valid([]byte(s)) {
				verrs.Add(name, "must be valid JSON", "invalid_json")
			}
		}
	default:
	}
}

func (v *FieldValidator) validateMedia(val interface{}, name string, verrs *interfaces.ValidationErrors) {
	if val == nil {
		return
	}
	switch m := val.(type) {
	case string:
		if m == "" {
			verrs.Add(name, "media reference must not be empty", "invalid_media")
		}
	case map[string]interface{}:
		if _, ok := m["id"]; !ok {
			if _, ok2 := m["url"]; !ok2 {
				verrs.Add(name, "media reference must have 'id' or 'url' field", "invalid_media")
			}
		}
	default:
		verrs.Add(name, "media must be a string or object", "type_mismatch")
	}
}

func (v *FieldValidator) validateRelation(ctx context.Context, contentType string, field *interfaces.Field, val interface{}, name string, verrs *interfaces.ValidationErrors) {
	if field.RelationConfig == nil {
		return
	}
	if field.RelationConfig.TargetContentType == "" {
		verrs.Add(name, "relation config missing target_content_type", "invalid_relation")
		return
	}

	if field.RelationConfig.TargetContentType == contentType {
		// Self-referencing is allowed for tree structures (e.g. menu parent, category parent).
		// Skip the target-exists check below since the target row is in the same table.
		return
	}

	if v.store == nil {
		return
	}

	switch id := val.(type) {
	case string:
		if id == "" {
			return
		}
		targetCT, err := v.store.GetContentType(ctx, field.RelationConfig.TargetContentType)
		if err != nil {
			return
		}
		_, err = v.store.GetContent(ctx, targetCT.TableName, id)
		if err != nil {
			verrs.Add(name, fmt.Sprintf("referenced %s item not found", field.RelationConfig.TargetContentType), "relation_not_found")
		}
	case []interface{}:
		if v.store == nil {
			return
		}
		targetCT, err := v.store.GetContentType(ctx, field.RelationConfig.TargetContentType)
		if err != nil {
			return
		}
		for _, item := range id {
			strID, ok := item.(string)
			if !ok || strID == "" {
				continue
			}
			_, err = v.store.GetContent(ctx, targetCT.TableName, strID)
			if err != nil {
				verrs.Add(name, fmt.Sprintf("referenced %s item %s not found", field.RelationConfig.TargetContentType, strID), "relation_not_found")
			}
		}
	}
}

func toInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		return strconv.Atoi(n)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toFloat64E(v interface{}) (float64, error) {
	f, ok := toFloat64(v)
	if !ok {
		return 0, fmt.Errorf("cannot convert to float64")
	}
	return f, nil
}
