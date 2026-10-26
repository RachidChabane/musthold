package scanreport_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/scanreport"
	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/format"
)

const verifiesTruncatedReport = " [verifies SP-2272~1]"

func TestTruncatedViewsRendered(t *testing.T) {
	r := hostReport(t)
	zero := format.ScanPromise{ID: "promise-zero", Quote: "A discount of zero percent leaves the total as it is.", Source: format.ScanSource{Name: format.ScanSourceReadme}}
	r.Promises = &scan.PromiseReport{
		Checked: true, Promises: []format.ScanPromise{zero}, Calls: 9, Budget: promise.Budget,
		Truncated: []scan.TruncatedView{{Promise: zero.ID, Shown: 12, Total: 40}, {Shown: 15, Total: 41}},
	}
	want := []string{
		`the code shown to the generation tickets for "A discount of zero percent leaves the total as it is." cut to 12 of 40 functions by the size bound of 48 KiB`,
		"the code shown to the labels ticket cut to 15 of 41 functions by the size bound of 48 KiB",
	}
	for name, rendered := range map[string]string{"text": scanreport.Text(r), "markdown": scanreport.Markdown(r)} {
		t.Run("the "+name+" report names each view the size bound cut and how much of it was shown"+verifiesTruncatedReport, func(t *testing.T) {
			for _, line := range want {
				if !strings.Contains(rendered, line) {
					t.Errorf("missing %q:\n%s", line, rendered)
				}
			}
		})
	}
}
