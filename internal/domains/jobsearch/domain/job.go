package domain

import "time"

type WorkMode string

const (
	WorkModeRemote  WorkMode = "remote"
	WorkModeHybrid  WorkMode = "hybrid"
	WorkModeOnsite  WorkMode = "onsite"
	WorkModeUnknown WorkMode = "unknown"
)

type RawJob struct {
	Source, ExternalID, URL, Title, Company, Description, Location string
	SalaryText, EmploymentType, WorkMode, ExperienceText           string
	Skills                                                         []string
	PublishedAt                                                    *time.Time
	FetchedAt                                                      time.Time
}

type NormalizedJob struct {
	ID, Source, ExternalID, CanonicalURL, Title, NormalizedTitle string
	Company, Description, City                                   string
	WorkMode                                                     WorkMode
	SalaryMin, SalaryMax                                         int64
	SalaryCurrency                                               string
	MinYearsExp, MaxYearsExp                                     int
	Skills                                                       []string
	ContentHash                                                  string
	SourcePublishedAt                                            *time.Time
	FirstSeenAt, LastSeenAt                                      time.Time
	EmploymentType                                               string
}

type SearchQuery struct {
	Keyword  string
	Location string
}

type Classification struct {
	JobID                        string
	Category, Seniority, Summary string
	AIRelevance, SkillMatch      float64
	MatchedSkills, MissingSkills []string
	HalalStatus, HalalReason     string
}

type MatchResult struct {
	Job            NormalizedJob
	Classification Classification
	FinalScore     float64
	SkillScore     float64
	RoleScore      float64
	WorkModeScore  float64
	SalaryScore    float64
	LocationScore  float64
}
