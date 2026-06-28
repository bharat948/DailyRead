package domain

// Interest represents a single user interest topic from the config.
type Interest struct {
	Tag       string   `json:"tag"`
	Primary   bool     `json:"primary"`
	Intensity string   `json:"intensity"`
	Types     []string `json:"types"`
}

// Candidate represents an article or resource discovered and vetted by the Researcher agent.
// The Curator enriches Why, How, and Slot before delivery.
type Candidate struct {
	InterestTag   string `json:"interest_tag"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Relevance     int    `json:"relevance"`       // 1-10 score of how well it matches the interest
	WordCount     int    `json:"word_count"`
	Why           string `json:"why"`             // personalized reason this item matters to the user
	How           string `json:"how"`             // how to read it (depth, time, focus points)
	Slot          string `json:"slot"`            // suggested reading slot: morning | evening | weekend
	ContentLength int    `json:"content_length"`
}
