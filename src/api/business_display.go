package main

import "advisor/scenario"

func (c config) demoSampleProducts() []string {
	return c.demoSampleProductsFor(c.scenarioName)
}

func (c config) demoSampleProductsFor(scenarioName string) []string {
	if labels, ok := scenario.SampleProductLabels(scenarioName); ok {
		return labels
	}
	return nil
}

func (c config) demoBusinessDisplay() (name, locality string) {
	return c.demoBusinessDisplayFor(c.scenarioName)
}

func (c config) demoBusinessDisplayFor(scenarioName string) (name, locality string) {
	if d, ok := scenario.DemoBusinessByID(scenarioName); ok {
		return d.BusinessName, d.Locality
	}
	baseDir := c.scenarioDataDir
	if baseDir == "" {
		baseDir, _ = scenario.FindDataDir("")
	}
	if baseDir == "" || scenarioName == "" {
		return scenarioName, ""
	}
	sf, err := scenario.Load(baseDir, scenarioName)
	if err != nil || len(sf.Locations) == 0 {
		return scenarioName, ""
	}
	loc := sf.Locations[0]
	name = loc.Name
	if loc.Address != nil {
		locality = loc.Address.Locality
	}
	return name, locality
}
