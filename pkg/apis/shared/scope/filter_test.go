package scope

import "testing"

func TestFrom(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]string
		wantWs *int64
		wantNs *int64
	}{
		{"empty", map[string]string{}, nil, nil},
		{"workspace only", map[string]string{"workspaceId": "1"}, ptr(1), nil},
		{"workspace + namespace", map[string]string{"workspaceId": "1", "namespaceId": "2"}, ptr(1), ptr(2)},
		{"namespace only (rare)", map[string]string{"namespaceId": "5"}, nil, ptr(5)},
		{"unrelated keys ignored", map[string]string{"pgsqlId": "3", "hostId": "9"}, nil, nil},
		{"empty string treated as missing", map[string]string{"workspaceId": ""}, nil, nil},
		{"invalid value treated as missing", map[string]string{"workspaceId": "abc"}, nil, nil},
		{"zero treated as missing", map[string]string{"workspaceId": "0"}, nil, nil},
		{"negative treated as missing", map[string]string{"workspaceId": "-1"}, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := From(c.params)
			if !ptrEq(f.WorkspaceID, c.wantWs) {
				t.Errorf("WorkspaceID: got %v want %v", deref(f.WorkspaceID), deref(c.wantWs))
			}
			if !ptrEq(f.NamespaceID, c.wantNs) {
				t.Errorf("NamespaceID: got %v want %v", deref(f.NamespaceID), deref(c.wantNs))
			}
		})
	}
}

func ptr(v int64) *int64 { return &v }
func ptrEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func deref(p *int64) any {
	if p == nil {
		return "nil"
	}
	return *p
}
