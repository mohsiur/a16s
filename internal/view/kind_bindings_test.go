package view

import "testing"

// TestBindKind_PaletteCoverage pins that every kind a user can reach via
// the `:` palette is bound through bindKind. Adding a new kind that wires
// up through bindKind in its init() will appear here automatically; if a
// kind is added without a binding, it stays invisible to the palette and
// this test does not change.
//
// We assert the legacy verb set keeps working — anything beyond this is
// extra (e.g. aliases). Adding a new entry here is a deliberate decision.
func TestBindKind_PaletteCoverage(t *testing.T) {
	cases := []struct {
		verb string
		want kind
	}{
		{"profiles", ProfileKind},
		{"clusters", ClusterKind},
		{"lambdas", LambdaKind},
		{"queues", SQSKind},
		{"sqs", SQSKind},
		{"tables", DynamoDBKind},
		{"ddb", DynamoDBKind},
		{"dynamodb", DynamoDBKind},
	}
	for _, c := range cases {
		got, ok := paletteVerbs[c.verb]
		if !ok {
			t.Errorf("paletteVerbs[%q] missing; want %v — bindKind must run from each kind's init()", c.verb, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("paletteVerbs[%q] = %v; want %v", c.verb, got, c.want)
		}
	}
}

// TestBindKind_RegistryNameCoverage pins the kind→registry-name mapping
// each kind file's init() declares. resolveResource consults this map, so
// dropping a binding silently turns the kind into "not Resource-aware" and
// every Resource-driven dispatch falls back to the legacy enum switch —
// hard to spot at runtime, easy to spot here.
func TestBindKind_RegistryNameCoverage(t *testing.T) {
	cases := []struct {
		k    kind
		want string
	}{
		{ClusterKind, "clusters"},
		{ServiceKind, "services"},
		{TaskKind, "tasks"},
		{ContainerKind, "containers"},
		{TaskDefinitionKind, "task-definitions"},
		{ServiceDeploymentKind, "service-deployments"},
		{LambdaKind, "lambda"},
		{SQSKind, "sqs"},
		{SQSPeekKind, "sqs-messages"},
		{DynamoDBKind, "ddb"},
		{DynamoDBIndexKind, "ddb-indexes"},
		{DynamoDBScanKind, "ddb-items"},
	}
	for _, c := range cases {
		got, ok := kindRegistryName[c.k]
		if !ok {
			t.Errorf("kindRegistryName[%v] missing; want %q", c.k, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("kindRegistryName[%v] = %q; want %q", c.k, got, c.want)
		}
	}
}
