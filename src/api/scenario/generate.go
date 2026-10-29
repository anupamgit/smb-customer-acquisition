package scenario

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const defaultOrderCount = 100

// Spec defines a generated business scenario.
type Spec struct {
	Name          string
	Label         string
	BusinessType  string
	LocationName  string
	Locality      string
	Region        string
	PostalCode    string
	Latitude      float64
	Longitude     float64
	Products      []productSpec
	OrderCount    int
	CustomerCount int
}

// DemoBusiness describes one built-in scenario for the owner-facing picker.
type DemoBusiness struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	BusinessName   string   `json:"businessName"`
	Locality       string   `json:"locality"`
	SampleProducts []string `json:"sampleProducts"`
}

type productSpec struct {
	Name       string
	PriceCents int64
	Weight     int // relative frequency
}

var builtin = map[string]Spec{
	"coffee-nyc": {
		Name: "coffee-nyc", Label: "Coffee shop", BusinessType: "coffee_shop",
		LocationName: "Harbor Lane Coffee", Locality: "New York", Region: "NY", PostalCode: "10001",
		Latitude: 40.7589, Longitude: -73.9851,
		Products: []productSpec{
			{Name: "Coffee", PriceCents: 450, Weight: 35},
			{Name: "Latte", PriceCents: 550, Weight: 25},
			{Name: "Espresso", PriceCents: 400, Weight: 15},
			{Name: "Croissant", PriceCents: 380, Weight: 15},
			{Name: "Muffin", PriceCents: 350, Weight: 10},
		},
		OrderCount: defaultOrderCount, CustomerCount: 40,
	},
	"salon-la": {
		Name: "salon-la", Label: "Salon", BusinessType: "hair_salon",
		LocationName: "Sunset Studio Salon", Locality: "Los Angeles", Region: "CA", PostalCode: "90028",
		Latitude: 34.1016, Longitude: -118.3267,
		Products: []productSpec{
			{Name: "Haircut", PriceCents: 6500, Weight: 30},
			{Name: "Hair Color", PriceCents: 12000, Weight: 20},
			{Name: "Blowout", PriceCents: 4500, Weight: 25},
			{Name: "Trim", PriceCents: 3500, Weight: 15},
			{Name: "Conditioning Treatment", PriceCents: 5500, Weight: 10},
		},
		OrderCount: defaultOrderCount, CustomerCount: 40,
	},
	"mexican-fast-austin": {
		Name: "mexican-fast-austin", Label: "Mexican fast food", BusinessType: "mexican_restaurant",
		LocationName: "Verde Taco Co.", Locality: "Austin", Region: "TX", PostalCode: "78701",
		Latitude: 30.2672, Longitude: -97.7431,
		Products: []productSpec{
			{Name: "Taco", PriceCents: 350, Weight: 30},
			{Name: "Burrito", PriceCents: 950, Weight: 25},
			{Name: "Quesadilla", PriceCents: 850, Weight: 20},
			{Name: "Chips & Salsa", PriceCents: 450, Weight: 15},
			{Name: "Horchata", PriceCents: 350, Weight: 10},
		},
		OrderCount: defaultOrderCount, CustomerCount: 40,
	},
}

// AllNames returns built-in scenario ids.
func AllNames() []string {
	return []string{"coffee-nyc", "salon-la", "mexican-fast-austin"}
}

// ValidName reports whether name is a built-in scenario id.
func ValidName(name string) bool {
	_, ok := builtin[name]
	return ok
}

// DemoBusinessByID returns one built-in demo entry for the owner picker.
func DemoBusinessByID(id string) (DemoBusiness, bool) {
	spec, ok := builtin[id]
	if !ok {
		return DemoBusiness{}, false
	}
	products, _ := SampleProductLabels(id)
	return DemoBusiness{
		ID: id, Label: spec.Label, BusinessName: spec.LocationName,
		Locality: spec.Locality, SampleProducts: products,
	}, true
}

// DemoBusinesses returns metadata for each built-in demo scenario (UI picker).
func DemoBusinesses() []DemoBusiness {
	return demoBusinessesForNames(AllNames())
}

func demoBusinessesForNames(names []string) []DemoBusiness {
	out := make([]DemoBusiness, 0, len(names))
	for _, name := range names {
		spec, ok := builtin[name]
		if !ok {
			continue
		}
		products, _ := SampleProductLabels(name)
		out = append(out, DemoBusiness{
			ID: name, Label: spec.Label, BusinessName: spec.LocationName,
			Locality: spec.Locality, SampleProducts: products,
		})
	}
	return out
}

// ProbeSpec is scenario metadata for live Qloo partner-discovery probes.
type ProbeSpec struct {
	Name           string
	BusinessType   string
	LocalityQuery  string
	Latitude       float64
	Longitude      float64
	ProductNames   []string
}

// ProbeSpecByName returns location and catalog fields for tooling and live probes.
func ProbeSpecByName(name string) (ProbeSpec, bool) {
	spec, ok := builtin[name]
	if !ok {
		return ProbeSpec{}, false
	}
	var products []string
	for _, p := range spec.Products {
		products = append(products, p.Name)
	}
	locality := spec.Locality
	if spec.Region != "" {
		locality = locality + ", " + spec.Region
	}
	return ProbeSpec{
		Name: spec.Name, BusinessType: spec.BusinessType, LocalityQuery: locality,
		Latitude: spec.Latitude, Longitude: spec.Longitude, ProductNames: products,
	}, true
}

// SampleProductLabels returns up to three product names for owner-facing wait copy.
func SampleProductLabels(name string) ([]string, bool) {
	spec, ok := builtin[name]
	if !ok {
		return nil, false
	}
	var out []string
	for _, p := range spec.Products {
		out = append(out, p.Name)
		if len(out) >= 3 {
			break
		}
	}
	return out, len(out) > 0
}

// Generate builds deterministic synthetic Square-shaped data for name.
func Generate(name string) (*SourceFile, error) {
	spec, ok := builtin[name]
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q (built-in: %v)", name, AllNames())
	}
	if spec.OrderCount <= 0 {
		spec.OrderCount = defaultOrderCount
	}
	if spec.CustomerCount <= 0 {
		spec.CustomerCount = 40
	}
	rng := rand.New(rand.NewSource(seed(name)))
	now := time.Now().UTC()

	loc := Location{
		ID:   "loc_" + name,
		Name: spec.LocationName,
		Address: &Address{
			AddressLine1: "100 Demo Street",
			Locality:     spec.Locality,
			AdminRegion:  spec.Region,
			PostalCode:   spec.PostalCode,
			Country:      "US",
		},
		Coordinates: &struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}{Latitude: spec.Latitude, Longitude: spec.Longitude},
	}

	customers := make([]Customer, spec.CustomerCount)
	for i := range customers {
		cid := fmt.Sprintf("cust_%s_%03d", name, i+1)
		customers[i] = Customer{
			ID: cid, GivenName: fmt.Sprintf("Patron%d", i+1), FamilyName: "Demo",
			CreatedAt: now.Add(-time.Duration(rng.Intn(400)+30) * 24 * time.Hour).Format(time.RFC3339),
			Address: &Address{
				AddressLine1: fmt.Sprintf("%d Demo Ave", 10+rng.Intn(900)),
				Locality:     spec.Locality, AdminRegion: spec.Region,
				PostalCode: spec.PostalCode, Country: "US",
			},
		}
	}

	// Weighted toward first ~12 customers (regulars).
	weights := make([]int, spec.CustomerCount)
	for i := range weights {
		if i < 12 {
			weights[i] = 5
		} else {
			weights[i] = 1
		}
	}

	orders := make([]Order, 0, spec.OrderCount)
	for o := 0; o < spec.OrderCount; o++ {
		custIdx := pickWeighted(rng, weights)
		prod := pickProduct(rng, spec.Products)
		qty := 1 + rng.Intn(2)
		lineTotal := prod.PriceCents * int64(qty)
		closed := now.Add(-time.Duration(rng.Intn(90)) * 24 * time.Hour).Add(-time.Duration(rng.Intn(86400)) * time.Second)
		orders = append(orders, Order{
			ID: fmt.Sprintf("ord_%s_%04d", name, o+1), CustomerID: customers[custIdx].ID,
			CreatedAt: closed.Add(-15 * time.Minute).Format(time.RFC3339),
			ClosedAt:  closed.Format(time.RFC3339),
			State:     "COMPLETED",
			TotalMoney: &Money{Amount: lineTotal, Currency: "USD"},
			LineItems: []LineItem{{
				Name: prod.Name, Quantity: fmt.Sprintf("%d", qty),
				TotalMoney: &Money{Amount: lineTotal, Currency: "USD"},
			}},
		})
	}

	return &SourceFile{
		Scenario: name, BusinessType: spec.BusinessType,
		Customers: customers, Orders: orders, Locations: []Location{loc},
	}, nil
}

func seed(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64())
}

func pickWeighted(rng *rand.Rand, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	n := rng.Intn(total)
	for i, w := range weights {
		n -= w
		if n < 0 {
			return i
		}
	}
	return 0
}

func pickProduct(rng *rand.Rand, products []productSpec) productSpec {
	total := 0
	for _, p := range products {
		total += p.Weight
	}
	n := rng.Intn(total)
	for _, p := range products {
		n -= p.Weight
		if n < 0 {
			return p
		}
	}
	return products[0]
}

// Write persists source.json under baseDir/name/.
func Write(baseDir, name string, sf *SourceFile) (string, error) {
	dir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := SourcePath(baseDir, name)
	b, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
