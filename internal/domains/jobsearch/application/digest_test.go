package application

import (
	"strings"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestFormatDigestCategorizesAndLabelsEveryMatch(t *testing.T) {
	matches := []domain.MatchResult{
		{Job: domain.NormalizedJob{Title: "Backend Engineer", Company: "PT Halal", CanonicalURL: "https://example.test/1", City: "Jakarta", WorkMode: domain.WorkModeHybrid, SalaryMin: 12_000_000, SalaryMax: 16_000_000, Skills: []string{"Go"}}, Classification: domain.Classification{HalalStatus: domain.HalalStatusHalal, Summary: "Sangat cocok."}, FinalScore: 94},
		{Job: domain.NormalizedJob{Title: "Web Engineer", Company: "Bank Contoh", CanonicalURL: "https://example.test/2"}, Classification: domain.Classification{HalalStatus: domain.HalalStatusNotHalal, HalalReason: "Perbankan ribawi"}, FinalScore: 76},
		{Job: domain.NormalizedJob{Title: "Frontend Engineer", Company: "PT Baru", CanonicalURL: "https://example.test/3"}, Classification: domain.Classification{HalalStatus: domain.HalalStatusNeedsReview}, FinalScore: 72},
	}
	digest := FormatDigest(matches)
	for _, expected := range []string{"Rekomendasi Utama", "PT Halal (Halal)", "Rp 12 jt–16 jt", "Lowongan Relevan Lainnya (2 lowongan)", "Bank Contoh (Tidak Halal: Perbankan ribawi)", "PT Baru (Perlu Riset)"} {
		if !strings.Contains(digest, expected) {
			t.Fatalf("missing %q in %q", expected, digest)
		}
	}
}
