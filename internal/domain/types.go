package domain

// Interest represents a single user interest topic from the config.
type Interest struct {
	Tag       string   `json:"tag"`
	Primary   bool     `json:"primary"`
	Intensity string   `json:"intensity"`
	Types     []string `json:"types"`
}

// Candidate represents an article or resource discovered and vetted by the Researcher agent.
type Candidate struct {
	InterestTag   string `json:"interest_tag"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Relevance     int    `json:"relevance"`      // 1-10 score of how well it matches the interest
	WordCount     int    `json:"word_count"`
	Why           string `json:"why"`            // Agent's reasoning for why this is a good fit
	ContentLength int    `json:"content_length"` // Approx characters or words
}
