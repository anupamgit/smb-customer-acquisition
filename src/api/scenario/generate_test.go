package scenario

import "testing"

func TestSampleProductLabels(t *testing.T) {
	labels, ok := SampleProductLabels("coffee-nyc")
	if !ok || len(labels) != 3 || labels[0] != "Coffee" {
		t.Fatalf("labels = %v ok=%v", labels, ok)
	}
}

func TestDemoBusinesses(t *testing.T) {
	demos := DemoBusinesses()
	if len(demos) != len(AllNames()) {
		t.Fatalf("demos=%d names=%d", len(demos), len(AllNames()))
	}
	if demos[0].ID != "coffee-nyc" || demos[0].Label == "" {
		t.Fatalf("first demo = %+v", demos[0])
	}
}

func TestGenerateHundredOrders(t *testing.T) {
	for _, name := range AllNames() {
		sf, err := Generate(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(sf.Orders) != 100 {
			t.Fatalf("%s: orders=%d want 100", name, len(sf.Orders))
		}
		if len(sf.Customers) < 20 {
			t.Fatalf("%s: too few customers", name)
		}
	}
}
