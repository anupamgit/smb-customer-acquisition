package main

import (
	"context"
	"fmt"

	"advisor/scenario"
)

type scenarioConnector struct {
	data *SquareSourceData
	name string
}

func scenarioDataDir(cfg config) (string, error) {
	baseDir := cfg.scenarioDataDir
	if baseDir == "" {
		var ok bool
		baseDir, ok = scenario.FindDataDir("")
		if !ok {
			return "", fmt.Errorf("scenario data directory not found (run make generate-scenarios from repo root)")
		}
	}
	return baseDir, nil
}

func newScenarioConnector(cfg config) (Connector, error) {
	return newScenarioConnectorNamed(cfg, cfg.scenarioName)
}

func newScenarioConnectorNamed(cfg config, name string) (Connector, error) {
	baseDir, err := scenarioDataDir(cfg)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = cfg.scenarioName
	}
	if !scenario.ValidName(name) {
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
	sf, err := scenario.Load(baseDir, name)
	if err != nil {
		return nil, err
	}
	return &scenarioConnector{name: name, data: toSquareSourceData(sf)}, nil
}

func (s *scenarioConnector) FetchSourceData(context.Context) (*SquareSourceData, error) {
	return s.data, nil
}

// ScenarioName identifies the built-in demo business for partner-discovery config lookup.
func (s *scenarioConnector) ScenarioName() string {
	return s.name
}

func toSquareSourceData(sf *scenario.SourceFile) *SquareSourceData {
	out := &SquareSourceData{
		Customers: make([]SquareCustomer, len(sf.Customers)),
		Orders:    make([]SquareOrder, len(sf.Orders)),
		Locations: make([]SquareLocation, len(sf.Locations)),
	}
	for i, c := range sf.Customers {
		out.Customers[i] = SquareCustomer{
			ID: c.ID, GivenName: c.GivenName, FamilyName: c.FamilyName,
			Email: c.Email, CreatedAt: c.CreatedAt, Address: toSquareAddr(c.Address),
		}
	}
	for i, o := range sf.Orders {
		ord := SquareOrder{
			ID: o.ID, CustomerID: o.CustomerID, CreatedAt: o.CreatedAt,
			ClosedAt: o.ClosedAt, State: o.State,
		}
		if o.TotalMoney != nil {
			ord.TotalMoney = &SquareMoney{Amount: o.TotalMoney.Amount, Currency: o.TotalMoney.Currency}
		}
		for _, li := range o.LineItems {
			item := SquareLineItem{Name: li.Name, Quantity: li.Quantity}
			if li.TotalMoney != nil {
				item.TotalMoney = &SquareMoney{Amount: li.TotalMoney.Amount, Currency: li.TotalMoney.Currency}
			}
			ord.LineItems = append(ord.LineItems, item)
		}
		out.Orders[i] = ord
	}
	for i, l := range sf.Locations {
		loc := SquareLocation{ID: l.ID, Name: l.Name, Address: toSquareAddr(l.Address)}
		if l.Coordinates != nil {
			loc.Coordinates = &struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			}{Latitude: l.Coordinates.Latitude, Longitude: l.Coordinates.Longitude}
		}
		out.Locations[i] = loc
	}
	return out
}

func toSquareAddr(a *scenario.Address) *SquareAddress {
	if a == nil {
		return nil
	}
	return &SquareAddress{
		AddressLine1: a.AddressLine1, Locality: a.Locality,
		AdminRegion: a.AdminRegion, PostalCode: a.PostalCode, Country: a.Country,
	}
}

func newBusinessConnector(cfg config) (Connector, error) {
	if cfg.useLiveConnector() {
		return newSquareClient(cfg), nil
	}
	return newScenarioConnector(cfg)
}

func (c config) useLiveConnector() bool {
	return c.liveConnectorEnabled
}

func (c config) scenarioDataReady() bool {
	return c.allScenarioDataReady()
}

func (c config) allScenarioDataReady() bool {
	if c.useLiveConnector() {
		return false
	}
	baseDir := c.scenarioDataDir
	if baseDir == "" {
		baseDir, _ = scenario.FindDataDir("")
	}
	if baseDir == "" {
		return false
	}
	for _, name := range scenario.AllNames() {
		if _, err := scenario.Load(baseDir, name); err != nil {
			return false
		}
	}
	return true
}
