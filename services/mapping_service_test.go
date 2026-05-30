package services

import "testing"

func TestQueryInt64(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
		want   int64
	}{
		{name: "empty", params: map[string]string{}, want: 0},
		{name: "invalid", params: map[string]string{"minDurationMs": "slow"}, want: 0},
		{name: "negative", params: map[string]string{"minDurationMs": "-1"}, want: 0},
		{name: "valid", params: map[string]string{"minDurationMs": "1500"}, want: 1500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryInt64(tt.params, "minDurationMs"); got != tt.want {
				t.Fatalf("queryInt64() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestQueryBool(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
		want   bool
		wantOK bool
	}{
		{name: "empty", params: map[string]string{}, want: false, wantOK: false},
		{name: "true", params: map[string]string{"reusedConn": "true"}, want: true, wantOK: true},
		{name: "one", params: map[string]string{"reusedConn": "1"}, want: true, wantOK: true},
		{name: "false", params: map[string]string{"reusedConn": "false"}, want: false, wantOK: true},
		{name: "zero", params: map[string]string{"reusedConn": "0"}, want: false, wantOK: true},
		{name: "invalid", params: map[string]string{"reusedConn": "maybe"}, want: false, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := queryBool(tt.params, "reusedConn")
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("queryBool() = (%t, %t), want (%t, %t)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
