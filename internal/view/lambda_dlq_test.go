package view

import "testing"

func TestParseQueueNameFromArn(t *testing.T) {
	cases := map[string]string{
		"arn:aws:sqs:us-east-1:111:my-dlq":      "my-dlq",
		"arn:aws:sqs:eu-west-2:222:another-dlq": "another-dlq",
		"":                                      "",
		"not-an-arn":                            "",
	}
	for in, want := range cases {
		if got := parseQueueNameFromArn(in); got != want {
			t.Fatalf("parseQueueNameFromArn(%q) = %q; want %q", in, got, want)
		}
	}
}
