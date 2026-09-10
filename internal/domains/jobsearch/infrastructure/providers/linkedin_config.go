package providers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func validateLinkedInURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("LinkedIn %s URL must be an absolute HTTP URL", name)
	}
	return nil
}

func normalizeLinkedInJobTypes(values []string) ([]string, error) {
	allowed := map[string]bool{"F": true, "P": true, "C": true, "T": true, "V": true, "I": true, "O": true}
	seen := make(map[string]bool)
	var normalized []string
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !allowed[value] {
			return nil, fmt.Errorf("unsupported LinkedIn job type %q", value)
		}
		if !seen[value] {
			seen[value] = true
			normalized = append(normalized, value)
		}
	}
	return normalized, nil
}

func normalizeLinkedInCompanyIDs(values []string) ([]string, error) {
	seen := make(map[string]bool)
	var normalized []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid LinkedIn company ID %q", value)
		}
		if !seen[value] {
			seen[value] = true
			normalized = append(normalized, value)
		}
	}
	return normalized, nil
}

func linkedInExperienceLevels(maxYears int) []string {
	switch {
	case maxYears <= 0:
		return nil
	case maxYears <= 1:
		return []string{"2"}
	case maxYears <= 3:
		return []string{"2", "3"}
	case maxYears <= 6:
		return []string{"2", "3", "4"}
	default:
		return []string{"3", "4", "5", "6"}
	}
}

func linkedInWorkModes(values []domain.WorkMode) []string {
	seen := make(map[string]bool)
	var modes []string
	for _, value := range values {
		code := ""
		switch value {
		case domain.WorkModeOnsite:
			code = "1"
		case domain.WorkModeRemote:
			code = "2"
		case domain.WorkModeHybrid:
			code = "3"
		}
		if code != "" && !seen[code] {
			seen[code] = true
			modes = append(modes, code)
		}
	}
	return modes
}

func linkedInQueries(criteria domain.Criteria, maxQueries int) []domain.SearchQuery {
	location := ""
	if len(criteria.Locations) > 0 {
		location = criteria.Locations[0]
	}
	seen := make(map[string]bool)
	var queries []domain.SearchQuery
	add := func(keyword string) {
		keyword = strings.TrimSpace(keyword)
		key := strings.ToLower(keyword + "|" + location)
		if keyword != "" && !seen[key] && len(queries) < maxQueries {
			seen[key] = true
			queries = append(queries, domain.SearchQuery{
				Keyword:   keyword,
				Location:  location,
				MaxYears:  criteria.MaxYears,
				WorkModes: append([]domain.WorkMode(nil), criteria.WorkModes...),
			})
		}
	}
	for _, position := range criteria.Positions {
		add(position)
	}
	for _, position := range criteria.Positions {
		for _, skill := range criteria.Skills {
			add(position + " " + skill)
		}
	}
	if len(queries) == 0 {
		add("software engineer")
	}
	return queries
}
