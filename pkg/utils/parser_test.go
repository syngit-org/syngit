package utils

import (
	"reflect"
	"testing"
)

func TestExcludedFieldsFromJson(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		path string
		want map[string]interface{}
	}{
		{
			name: "dotted path removes leaf",
			data: map[string]interface{}{
				"a": map[string]interface{}{"b": "value", "c": "keep"},
			},
			path: "a.b",
			want: map[string]interface{}{
				"a": map[string]interface{}{"c": "keep"},
			},
		},
		{
			name: "leading dot is tolerated",
			data: map[string]interface{}{"test3": "value"},
			path: ".test3",
			want: map[string]interface{}{},
		},
		{
			name: "bracket notation preserves dots and special chars in key",
			data: map[string]interface{}{
				"test4": map[string]interface{}{
					"this.string-is:the/same*key": map[string]interface{}{
						"test5": map[string]interface{}{
							"test6": "value",
							"keep":  "stays",
						},
					},
				},
			},
			path: ".test4[this.string-is:the/same*key].test5[test6]",
			want: map[string]interface{}{
				"test4": map[string]interface{}{
					"this.string-is:the/same*key": map[string]interface{}{
						"test5": map[string]interface{}{
							"keep": "stays",
						},
					},
				},
			},
		},
		{
			name: "missing intermediate key is a no-op",
			data: map[string]interface{}{
				"a": map[string]interface{}{"b": "value"},
			},
			path: "a.missing.x",
			want: map[string]interface{}{
				"a": map[string]interface{}{"b": "value"},
			},
		},
		{
			name: "non-map intermediate is a no-op",
			data: map[string]interface{}{
				"a": "not-a-map",
			},
			path: "a.b",
			want: map[string]interface{}{
				"a": "not-a-map",
			},
		},
		{
			name: "empty path is a no-op",
			data: map[string]interface{}{"a": "v"},
			path: "",
			want: map[string]interface{}{"a": "v"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ExcludedFieldsFromJson(tc.data, tc.path)
			if !reflect.DeepEqual(tc.data, tc.want) {
				t.Errorf("after ExcludedFieldsFromJson, data=%#v, want %#v", tc.data, tc.want)
			}
		})
	}
}
