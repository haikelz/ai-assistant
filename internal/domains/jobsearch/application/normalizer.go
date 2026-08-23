package application

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

var salaryNumber = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(jt|juta|mio|million|k|rb|ribu)?`)
var experienceRange = regexp.MustCompile(`(?i)(\d+)\s*(?:-|–|to|sampai)?\s*(\d+)?\s*(?:tahun|years?)`)

func NormalizeJob(raw domain.RawJob, now time.Time) domain.NormalizedJob {
	source := strings.ToLower(strings.TrimSpace(raw.Source))
	canonicalURL := canonicalizeURL(raw.URL)
	externalID := strings.TrimSpace(raw.ExternalID)
	if externalID == "" {
		sum := sha256.Sum256([]byte(strings.ToLower(canonicalURL)))
		externalID = hex.EncodeToString(sum[:8])
	}
	minimumSalary, maximumSalary, currency := parseSalary(raw.SalaryText)
	minimumYears, maximumYears := parseExperience(raw.ExperienceText)
	job := domain.NormalizedJob{
		ID: source + ":" + externalID, Source: source, ExternalID: externalID,
		CanonicalURL: canonicalURL, Title: cleanText(raw.Title), NormalizedTitle: strings.ToLower(cleanText(raw.Title)),
		Company: cleanText(raw.Company), Description: cleanText(raw.Description), City: cleanText(raw.Location),
		WorkMode: normalizeWorkMode(raw.WorkMode + " " + raw.Location), SalaryMin: minimumSalary, SalaryMax: maximumSalary,
		SalaryCurrency: currency, MinYearsExp: minimumYears, MaxYearsExp: maximumYears,
		Skills: normalizeSkills(raw.Skills), SourcePublishedAt: raw.PublishedAt, FirstSeenAt: now.UTC(), LastSeenAt: now.UTC(),
		EmploymentType: cleanText(raw.EmploymentType),
	}
	job.ContentHash = GenerateContentHash(job)
	return job
}

func GenerateContentHash(job domain.NormalizedJob) string {
	data := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(job.Title)), strings.ToLower(strings.TrimSpace(job.Company)),
		strings.ToLower(strings.TrimSpace(job.City)), strconv.FormatInt(job.SalaryMin, 10), strconv.FormatInt(job.SalaryMax, 10),
		string(job.WorkMode), strings.Join(normalizeSkills(job.Skills), ","), strings.Join(strings.Fields(job.Description), " "),
	}, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func canonicalizeURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return strings.TrimSpace(rawURL)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func cleanText(value string) string { return strings.Join(strings.Fields(value), " ") }

func normalizeSkills(skills []string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, skill := range skills {
		skill = strings.ToLower(cleanText(skill))
		if skill != "" && !seen[skill] {
			seen[skill] = true
			normalized = append(normalized, skill)
		}
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeWorkMode(value string) domain.WorkMode {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "remote") || strings.Contains(value, "work from home") || strings.Contains(value, "wfh"):
		return domain.WorkModeRemote
	case strings.Contains(value, "hybrid"):
		return domain.WorkModeHybrid
	case strings.Contains(value, "onsite") || strings.Contains(value, "on-site") || strings.Contains(value, "office"):
		return domain.WorkModeOnsite
	default:
		return domain.WorkModeUnknown
	}
}

func parseSalary(value string) (int64, int64, string) {
	matches := salaryNumber.FindAllStringSubmatch(strings.ToLower(value), 2)
	if len(matches) == 0 {
		return 0, 0, ""
	}
	parse := func(match []string, inheritedUnit string) int64 {
		number, _ := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64)
		multiplier := float64(1)
		unit := match[2]
		if unit == "" {
			unit = inheritedUnit
		}
		switch unit {
		case "jt", "juta", "mio", "million":
			multiplier = 1_000_000
		case "k", "rb", "ribu":
			multiplier = 1_000
		}
		return int64(number * multiplier)
	}
	unit := matches[0][2]
	if unit == "" && len(matches) > 1 {
		unit = matches[1][2]
	}
	minimum := parse(matches[0], unit)
	maximum := minimum
	if len(matches) > 1 {
		maximum = parse(matches[1], unit)
	}
	return minimum, maximum, "IDR"
}

func parseExperience(value string) (int, int) {
	match := experienceRange.FindStringSubmatch(value)
	if len(match) < 2 {
		return 0, 0
	}
	minimum, _ := strconv.Atoi(match[1])
	maximum := minimum
	if len(match) > 2 && match[2] != "" {
		maximum, _ = strconv.Atoi(match[2])
	}
	return minimum, maximum
}
