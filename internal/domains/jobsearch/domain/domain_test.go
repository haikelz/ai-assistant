package domain

import (
	"strings"
	"testing"
)

func TestParseFilterFormatAndSplit(t *testing.T) {
	c := ParseQuery("Software Engineer | go,typescript | 1-3 | Jakarta Selatan | halal")
	if !c.Halal || c.MaxYears != 3 || !c.Interactive {
		t.Fatalf("criteria = %#v", c)
	}
	jobs := []Job{{Title: "Software Developer", Skills: "Golang", Location: "South Jakarta", Company: "A"}, {Title: "Engineer", Skills: "Java", Location: "Bandung"}}
	got := FilterAndSort(jobs, c, 20)
	if len(got) != 1 || got[0].Company != "A" {
		t.Fatalf("filtered = %#v", got)
	}
	message := FormatMessage("Hasil", Result{Kitalulus: got})
	if !strings.Contains(message, "A. Kitalulus") || !strings.Contains(message, "B. Dealls") {
		t.Fatal(message)
	}
	long := strings.Repeat("lowongan kerja\n", 400)
	chunks := SplitTelegramMessage(long, 4000)
	if strings.Join(chunks, "") != long {
		t.Fatal("chunks lost content")
	}
	for _, v := range chunks {
		if len(v) > 4000 {
			t.Fatal("oversized chunk")
		}
	}
}
