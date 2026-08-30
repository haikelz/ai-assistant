package providers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
	xhtml "golang.org/x/net/html"
)

func parseLinkedInSearch(body []byte) ([]domain.RawJob, error) {
	document, err := xhtml.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	var jobs []domain.RawJob
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			if urn := nodeAttribute(node, "data-entity-urn"); strings.Contains(urn, "jobPosting:") {
				if job, ok := linkedInJobFromNode(node, urn); ok {
					jobs = append(jobs, job)
				}
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if len(jobs) == 0 {
		lowerBody := strings.ToLower(string(body))
		if strings.Contains(lowerBody, "base-search-card") || strings.Contains(lowerBody, "job-search-card") {
			return nil, domain.ErrProviderLayout
		}
	}
	return jobs, nil
}

func linkedInJobFromNode(node *xhtml.Node, urn string) (domain.RawJob, bool) {
	externalID := urn[strings.LastIndex(urn, ":")+1:]
	title := nodeText(findNodeByClass(node, "base-search-card__title"))
	company := nodeText(findNodeByClass(node, "base-search-card__subtitle"))
	location := nodeText(findNodeByClass(node, "job-search-card__location"))
	linkNode := findNodeByClass(node, "base-card__full-link")
	jobURL := nodeAttribute(linkNode, "href")
	if jobURL == "" {
		jobURL = findLinkedInJobURL(node)
	}
	if externalID == "" {
		externalID = linkedInJobIDFromURL(jobURL)
	}
	if externalID == "" || title == "" || jobURL == "" {
		return domain.RawJob{}, false
	}
	var publishedAt *time.Time
	if timeNode := findNodeByTag(node, "time"); timeNode != nil {
		publishedAt = parseLinkedInTime(nodeAttribute(timeNode, "datetime"))
	}
	return domain.RawJob{Source: "linkedin", ExternalID: externalID, URL: jobURL, Title: title, Company: company, Location: location, PublishedAt: publishedAt, FetchedAt: time.Now().UTC()}, true
}

func enrichLinkedInJob(job *domain.RawJob, body []byte) error {
	document, err := xhtml.Parse(strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("parse HTML: %w", err)
	}
	descriptionNode := findNodeByClass(document, "show-more-less-html__markup")
	if descriptionNode == nil {
		descriptionNode = findNodeByClass(document, "description__text")
	}
	job.Description = nodeText(descriptionNode)
	job.SalaryText = nodeText(findNodeByClass(document, "compensation__salary"))
	criteria := linkedInCriteria(document)
	job.EmploymentType = criteria["employment type"]
	job.ExperienceText = criteria["seniority level"]
	if experience := linkedInExperienceText(job.Description); experience != "" {
		job.ExperienceText = strings.TrimSpace(job.ExperienceText + " " + experience)
	}
	job.WorkMode = linkedInWorkModeText(job.Description + " " + job.Location)
	if job.Description == "" && len(criteria) == 0 {
		return domain.ErrProviderLayout
	}
	return nil
}

func linkedInCriteria(document *xhtml.Node) map[string]string {
	criteria := make(map[string]string)
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if hasNodeClass(node, "description__job-criteria-item") {
			label := strings.ToLower(nodeText(findNodeByTag(node, "h3")))
			value := nodeText(findNodeByClass(node, "description__job-criteria-text"))
			if label != "" && value != "" {
				criteria[label] = value
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return criteria
}

func linkedInLegacyJobs(rawJobs []domain.RawJob) []domain.Job {
	jobs := make([]domain.Job, 0, len(rawJobs))
	for _, raw := range rawJobs {
		minimumYears, maximumYears := linkedInYears(raw.ExperienceText)
		postedAt := ""
		if raw.PublishedAt != nil {
			postedAt = raw.PublishedAt.Format(time.RFC3339)
		}
		jobs = append(jobs, domain.Job{Title: raw.Title, Company: raw.Company, Location: raw.Location, URL: raw.URL, Source: raw.Source, Salary: raw.SalaryText, Type: raw.EmploymentType, Experience: raw.ExperienceText, Skills: strings.Join(raw.Skills, ", "), PostedAt: postedAt, MinYearsExp: minimumYears, MaxYearsExp: maximumYears})
	}
	return jobs
}

var linkedInIDPattern = regexp.MustCompile(`-(\d+)(?:\?|$)`)
var linkedInExperiencePattern = regexp.MustCompile(`(?i)\b\d+\s*(?:-|–|to|sampai)?\s*\d*\s*(?:years?|tahun)\b`)
var linkedInNumberPattern = regexp.MustCompile(`\d+`)

func linkedInJobIDFromURL(value string) string {
	match := linkedInIDPattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func linkedInExperienceText(value string) string {
	return strings.Join(strings.Fields(linkedInExperiencePattern.FindString(value)), " ")
}

func linkedInYears(value string) (int, int) {
	match := linkedInExperiencePattern.FindString(value)
	numbers := linkedInNumberPattern.FindAllString(match, 2)
	if len(numbers) == 0 {
		return 0, 0
	}
	minimum, _ := strconv.Atoi(numbers[0])
	maximum := minimum
	if len(numbers) == 2 {
		maximum, _ = strconv.Atoi(numbers[1])
	}
	return minimum, maximum
}

func linkedInWorkModeText(value string) string {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "remote") || strings.Contains(value, "work from home"):
		return "remote"
	case strings.Contains(value, "hybrid"):
		return "hybrid"
	case strings.Contains(value, "on-site") || strings.Contains(value, "onsite"):
		return "onsite"
	default:
		return ""
	}
}

func findNodeByClass(root *xhtml.Node, class string) *xhtml.Node {
	if root == nil {
		return nil
	}
	if hasNodeClass(root, class) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findNodeByClass(child, class); found != nil {
			return found
		}
	}
	return nil
}

func findNodeByTag(root *xhtml.Node, tag string) *xhtml.Node {
	if root == nil {
		return nil
	}
	if root.Type == xhtml.ElementNode && root.Data == tag {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findNodeByTag(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func findLinkedInJobURL(root *xhtml.Node) string {
	if root == nil {
		return ""
	}
	if root.Type == xhtml.ElementNode && root.Data == "a" {
		if href := nodeAttribute(root, "href"); strings.Contains(href, "/jobs/view/") {
			return href
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if value := findLinkedInJobURL(child); value != "" {
			return value
		}
	}
	return ""
}

func hasNodeClass(node *xhtml.Node, class string) bool {
	for _, value := range strings.Fields(nodeAttribute(node, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func nodeAttribute(node *xhtml.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func nodeText(node *xhtml.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func parseLinkedInTime(value string) *time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return &parsed
		}
	}
	return nil
}
