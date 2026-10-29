package scenario

// SourceFile matches the JSON persisted for the scenario connector (same
// shape as SquareSourceData in the main package).
type SourceFile struct {
	Scenario     string     `json:"scenario"`
	BusinessType string     `json:"businessType"`
	Customers    []Customer `json:"customers"`
	Orders       []Order    `json:"orders"`
	Locations    []Location `json:"locations"`
}

type Customer struct {
	ID         string   `json:"id"`
	GivenName  string   `json:"given_name"`
	FamilyName string   `json:"family_name"`
	Email      string   `json:"email_address,omitempty"`
	CreatedAt  string   `json:"created_at"`
	Address    *Address `json:"address"`
}

type Address struct {
	AddressLine1 string `json:"address_line_1"`
	Locality     string `json:"locality"`
	AdminRegion  string `json:"administrative_district_level_1"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`
}

type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type LineItem struct {
	Name       string `json:"name"`
	Quantity   string `json:"quantity"`
	TotalMoney *Money `json:"total_money,omitempty"`
}

type Order struct {
	ID         string     `json:"id"`
	CustomerID string     `json:"customer_id"`
	CreatedAt  string     `json:"created_at"`
	ClosedAt   string     `json:"closed_at"`
	State      string     `json:"state"`
	TotalMoney *Money     `json:"total_money"`
	LineItems  []LineItem `json:"line_items"`
}

type Location struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Address     *Address `json:"address"`
	Coordinates *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinates"`
}
