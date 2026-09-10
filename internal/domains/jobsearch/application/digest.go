package application

import (
	"fmt"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func FormatDigest(matches []domain.MatchResult) string {
	var builder strings.Builder
	builder.WriteString("Selamat pagi! ☀️ Berikut update lowongan kerja terkurasi hari ini:\n\n")
	if len(matches) == 0 {
		builder.WriteString("Belum ada lowongan baru yang mencapai match minimum hari ini.\n")
		builder.WriteString("\n— Dikirim otomatis oleh Job Alert Bot")
		return builder.String()
	}

	top := make([]domain.MatchResult, 0, len(matches))
	other := make([]domain.MatchResult, 0, len(matches))
	for _, match := range matches {
		if match.FinalScore >= 85 {
			top = append(top, match)
		} else {
			other = append(other, match)
		}
	}
	if len(top) > 0 {
		builder.WriteString("🔥 Rekomendasi Utama (Match ≥ 85%)\n\n")
		for index, match := range top {
			writeDetailedMatch(&builder, index+1, match)
		}
	}
	if len(other) > 0 {
		fmt.Fprintf(&builder, "📌 Lowongan Relevan Lainnya (%d lowongan)\n\n", len(other))
		for _, match := range other {
			fmt.Fprintf(
				&builder,
				"- %s — %s%s (Match %.0f%%)\n",
				cleanLine(match.Job.Title),
				cleanLine(match.Job.Company),
				halalLabelForMatch(match.Classification),
				match.FinalScore,
			)
			fmt.Fprintf(&builder, "  🔗 %s\n", match.Job.CanonicalURL)
		}
	}
	builder.WriteString("\n— Dikirim otomatis oleh Job Alert Bot")
	return builder.String()
}

func writeDetailedMatch(builder *strings.Builder, number int, match domain.MatchResult) {
	fmt.Fprintf(builder, "%d. %s — %s%s\n", number, cleanLine(match.Job.Title), cleanLine(match.Job.Company), halalLabelForMatch(match.Classification))
	details := []string{fmt.Sprintf("⭐ Match: %.0f%%", match.FinalScore)}
	location := cleanLine(match.Job.City)
	if match.Job.WorkMode != domain.WorkModeUnknown {
		if location == "" {
			location = titleWorkMode(match.Job.WorkMode)
		} else {
			location += " (" + titleWorkMode(match.Job.WorkMode) + ")"
		}
	}
	if location != "" {
		details = append(details, "📍 "+location)
	}
	if salary := formatSalary(match.Job); salary != "" {
		details = append(details, "💰 "+salary)
	}
	builder.WriteString("   " + strings.Join(details, " | ") + "\n")
	if len(match.Job.Skills) > 0 {
		builder.WriteString("   🛠 " + strings.Join(match.Job.Skills, ", ") + "\n")
	}
	if summary := cleanLine(match.Classification.Summary); summary != "" {
		builder.WriteString("   💡 " + summary + "\n")
	}
	if reason := cleanLine(match.Classification.HalalReason); reason != "" {
		builder.WriteString("   🕌 " + reason + "\n")
	}
	builder.WriteString("   🔗 " + match.Job.CanonicalURL + "\n\n")
}

func halalLabelForMatch(classification domain.Classification) string {
	switch classification.HalalStatus {
	case domain.HalalStatusHalal:
		return " (Halal)"
	case domain.HalalStatusNotHalal:
		if reason := cleanLine(classification.HalalReason); reason != "" {
			return " (Tidak Halal: " + reason + ")"
		}
		return " (Tidak Halal)"
	default:
		return " (Perlu Riset)"
	}
}

func formatSalary(job domain.NormalizedJob) string {
	if job.SalaryMin == 0 && job.SalaryMax == 0 {
		return ""
	}
	currency := job.SalaryCurrency
	if currency == "" || strings.EqualFold(currency, "IDR") {
		currency = "Rp"
	}
	if job.SalaryMin > 0 && job.SalaryMax > 0 {
		return fmt.Sprintf("%s %s–%s", currency, shortAmount(job.SalaryMin), shortAmount(job.SalaryMax))
	}
	if job.SalaryMin > 0 {
		return fmt.Sprintf("%s %s+", currency, shortAmount(job.SalaryMin))
	}
	return fmt.Sprintf("hingga %s %s", currency, shortAmount(job.SalaryMax))
}

func shortAmount(amount int64) string {
	if amount >= 1_000_000 && amount%1_000_000 == 0 {
		return fmt.Sprintf("%d jt", amount/1_000_000)
	}
	if amount >= 1_000_000 {
		return fmt.Sprintf("%.1f jt", float64(amount)/1_000_000)
	}
	if amount >= 1_000 && amount%1_000 == 0 {
		return fmt.Sprintf("%d rb", amount/1_000)
	}
	return fmt.Sprintf("%d", amount)
}

func titleWorkMode(mode domain.WorkMode) string {
	value := string(mode)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func cleanLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
