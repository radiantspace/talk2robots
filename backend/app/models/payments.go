package models

type UsageThresholds struct {
	Thresholds []UsageThreshold
}

type UsageThreshold struct {
	Percentage float64
	Message    string
}
