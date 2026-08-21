package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	HalalStatusHalal       = "halal"
	HalalStatusNotHalal    = "tidak_halal"
	HalalStatusNeedsReview = "perlu_riset"
)

type Job struct {
	Title, Company, Location, URL, Source, Salary, Type, Experience, Skills, PostedAt string
	MinYearsExp, MaxYearsExp                                                          int
	HalalStatus, HalalReason                                                          string
}

type Criteria struct {
	Positions, Skills, Locations []string
	MaxYears                     int
	Halal, Interactive           bool
}

type Result struct{ Kitalulus, Dealls []Job }

func ParseQuery(raw string) Criteria {
	p := strings.Split(raw, "|")
	for len(p) < 5 {
		p = append(p, "")
	}
	c := Criteria{Positions: split(p[0]), Skills: split(p[1]), MaxYears: ParseMaxYears(p[2]), Locations: split(p[3]), Interactive: true}
	h := strings.ToLower(strings.TrimSpace(p[4]))
	c.Halal = h == "halal" || h == "true" || h == "ya" || h == "yes" || h == "1"
	return c
}

func split(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func ParseMaxYears(s string) int {
	p := strings.Split(strings.TrimSpace(s), "-")
	n, _ := strconv.Atoi(strings.TrimSpace(p[len(p)-1]))
	return n
}

func FilterAndSort(jobs []Job, c Criteria, limit int) []Job {
	out := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		if len(c.Locations) > 0 && !locationMatch(j.Location, c.Locations) {
			continue
		}
		if c.MaxYears > 0 && j.MinYearsExp > c.MaxYears {
			continue
		}
		if len(c.Positions) > 0 && !termsMatch(strings.ToLower(j.Title), c.Positions) {
			continue
		}
		if len(c.Skills) > 0 && strings.TrimSpace(j.Skills) != "" && !termsMatch(strings.ToLower(j.Skills), c.Skills) {
			continue
		}
		out = append(out, j)
	}
	sort.SliceStable(out, func(i, j int) bool { return relevance(out[i], c.Skills) > relevance(out[j], c.Skills) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func termsMatch(text string, terms []string) bool {
	for _, t := range terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && strings.Contains(text, t) {
			return true
		}
		for _, w := range strings.Fields(t) {
			if len(w) > 2 && strings.Contains(text, w) {
				return true
			}
		}
	}
	return false
}
func normalizeLocation(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer("south jakarta", "jakarta selatan", "central jakarta", "jakarta pusat", "north jakarta", "jakarta utara", "west jakarta", "jakarta barat", "east jakarta", "jakarta timur", "jakarta raya", "jakarta")
	return r.Replace(s)
}
func locationMatch(s string, locations []string) bool {
	s = normalizeLocation(s)
	for _, v := range locations {
		if strings.Contains(s, normalizeLocation(v)) {
			return true
		}
	}
	return false
}
func relevance(j Job, skills []string) int {
	n := 0
	text := strings.ToLower(j.Skills + " " + j.Title)
	for _, s := range skills {
		if strings.Contains(text, strings.ToLower(s)) {
			n++
		}
	}
	return n
}

func FormatMessage(greeting string, result Result) string {
	var b strings.Builder
	b.WriteString(greeting + "\n\nDaftar Job Terbaru:\n\n")
	writeSection(&b, "A. Kitalulus", result.Kitalulus)
	writeSection(&b, "B. Dealls", result.Dealls)
	b.WriteString("\n— Dikirim otomatis oleh Job Alert Bot")
	return b.String()
}
func writeSection(b *strings.Builder, h string, jobs []Job) {
	b.WriteString(h + "\n\n")
	if len(jobs) == 0 {
		b.WriteString("Tidak ada lowongan baru yang sesuai kriteria.\n\n")
		return
	}
	for i, j := range jobs {
		fmt.Fprintf(b, "%d. **%s**\n", i+1, j.Title)
		if j.Company != "" {
			fmt.Fprintf(b, "   🏢 %s%s\n", j.Company, halalLabel(j))
		}
		if j.Location != "" {
			fmt.Fprintf(b, "   📍 %s\n", j.Location)
		}
		if j.Salary != "" {
			fmt.Fprintf(b, "   💰 %s\n", j.Salary)
		}
		if j.Type != "" {
			fmt.Fprintf(b, "   📋 %s\n", j.Type)
		}
		if j.Experience != "" {
			fmt.Fprintf(b, "   🎯 %s\n", j.Experience)
		}
		if j.Skills != "" {
			fmt.Fprintf(b, "   🛠 %s\n", j.Skills)
		}
		fmt.Fprintf(b, "   🔗 %s\n\n", j.URL)
	}
	b.WriteString("\n")
}
func halalLabel(j Job) string {
	switch j.HalalStatus {
	case HalalStatusHalal:
		return " (Halal)"
	case HalalStatusNotHalal:
		if j.HalalReason != "" {
			return " (Tidak Halal — " + j.HalalReason + ")"
		}
		return " (Tidak Halal)"
	case HalalStatusNeedsReview:
		return " (Perlu Riset)"
	}
	return ""
}

func SplitTelegramMessage(message string, max int) []string {
	if max <= 0 {
		return nil
	}
	var out []string
	for len(message) > max {
		cut := strings.LastIndex(message[:max], "\n\n")
		if cut < max/2 {
			cut = strings.LastIndex(message[:max], "\n")
		}
		if cut < 1 {
			cut = max
		}
		out = append(out, message[:cut])
		message = message[cut:]
	}
	if message != "" {
		out = append(out, message)
	}
	return out
}
