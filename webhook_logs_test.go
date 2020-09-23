//go:generate jsonenums -type=RequestStatus
package webhookrelay

import "testing"

func Test_getQuery(t *testing.T) {
	type args struct {
		options *WebhookLogsListOptions
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "nothing",
			args: args{options: &WebhookLogsListOptions{}},
			want: "",
		},
		{
			name: "limit",
			args: args{options: &WebhookLogsListOptions{
				Limit: 100,
			}},
			want: "limit=100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getQuery(tt.args.options); got != tt.want {
				t.Errorf("getQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
