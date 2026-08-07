import re

with open("internal/view/templates.go", "r") as f:
    content = f.read()

helper = """
		"val": func(v any) any {
			if v == nil {
				return nil
			}
			import_reflect_not_needed_if_we_use_fmt := False
			// actually let's use reflect
			val := reflect.ValueOf(v)
			if val.Kind() == reflect.Ptr {
				if val.IsNil() {
					return nil
				}
				return val.Elem().Interface()
			}
			return v
		},
		"eqStr": func(a, b any) bool {
			valA := a
			valB := b
			
			// Quick reflection to dereference
			if a != nil {
				va := reflect.ValueOf(a)
				if va.Kind() == reflect.Ptr && !va.IsNil() {
					valA = va.Elem().Interface()
				}
			}
			if b != nil {
				vb := reflect.ValueOf(b)
				if vb.Kind() == reflect.Ptr && !vb.IsNil() {
					valB = vb.Elem().Interface()
				}
			}
			
			return fmt.Sprintf("%v", valA) == fmt.Sprintf("%v", valB)
		},
"""

# Let's insert it after the "deref" helper
if "eqStr" not in content:
    content = content.replace('"deref": func(s *string) string {', helper + '\n\t\t"deref": func(s *string) string {')
    
    # Check if reflect is imported
    if '"reflect"' not in content:
        content = content.replace('"strings"', '"strings"\n\t"reflect"')

with open("internal/view/templates.go", "w") as f:
    f.write(content)

print("Added helpers to templates.go")
