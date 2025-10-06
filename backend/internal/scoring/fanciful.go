package scoring

import "strings"

// FancifulDecider implements xml.FancifulDecider using the seed list.
type FancifulDecider struct {
	seeds map[string]struct{}
}

// NewFancifulDecider constructs a decider from the provided seeds.
func NewFancifulDecider(seedPath string) (*FancifulDecider, error) {
	seeds, err := loadSeeds(seedPath)
	if err != nil {
		return nil, err
	}
	return &FancifulDecider{seeds: seeds}, nil
}

// Decide marks entries optionally fanciful using seeds and heuristics.
func (d *FancifulDecider) Decide(markNormalized string, classes []string, owner string) bool {
	key := strings.ReplaceAll(strings.ToLower(markNormalized), " ", "")
	key = strings.ReplaceAll(key, "-", "")
	if _, ok := d.seeds[key]; ok {
		return true
	}
	if len(markNormalized) >= 6 && len(classes) >= 2 {
		return true
	}
	return false
}
