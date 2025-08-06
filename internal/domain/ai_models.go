package domain

type ChapterStats struct {
	Total    int     `json:"total"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

// RecommendationInput is the data fed into our function.
type RecommendationInput struct {
	Subject  string                  `json:"subject"`
	Chapters map[string]ChapterStats `json:"chapters"`
}

type PracticeStep struct {
	Timeframe string `json:"timeframe"` // e.g., "Week 1", "Daily"
	Focus     string `json:"focus"`
}
type ResourceItem struct {
	Name     string `json:"name"`
	Topic    string `json:"topic"`
	Type     string `json:"type"`
	Platform string `json:"platform"` // <-- THE NEW FIELD
}
type Recommendations struct {
	Recommendations []string       `json:"recommendations"`
	Strategies      []string       `json:"strategies"`
	Resources       []ResourceItem `json:"resources"`
	PracticePlan    []PracticeStep `json:"practicePlan"`
} 