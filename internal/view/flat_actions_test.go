package view

import (
	"testing"
)

// TestFlatKindLegendsAdvertiseActions locks in that each flat kind's view
// keymap exposes its action keys to buildHeaderFlex. If a key disappears
// from a kind's `keys` slice the on-screen legend silently drops it, so
// this catches accidental regressions when editing the keymaps.
func TestFlatKindLegendsAdvertiseActions(t *testing.T) {
	app, err := newApp(Option{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	cases := []struct {
		name string
		keys []keyDescriptionPair
		want []string
	}{
		{
			name: "lambda",
			keys: newLambdaView(nil, app).keys,
			want: []string{"shift-l", "i", "shift-d"},
		},
		{
			name: "sqs",
			keys: newSQSView(nil, nil, app).keys,
			want: []string{"enter", "p", "s"},
		},
		{
			name: "ddb-index",
			keys: newDDBIndexView("t", nil, app).keys,
			want: []string{"enter", "q"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[string]bool{}
			for _, k := range tc.keys {
				seen[k.key] = true
			}
			for _, w := range tc.want {
				if !seen[w] {
					t.Errorf("legend missing key %q (had %v)", w, tc.keys)
				}
			}
		})
	}
}
