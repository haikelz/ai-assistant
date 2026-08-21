# AI Assistant Overview

AI Assistant combines three domains:

- **finance:** records, totals, workbook recap, local SQLite persistence, and
  optional spreadsheet synchronization;
- **jobsearch:** collects and filters job listings and can deliver results;
- **AI/provider:** proxies or invokes configured AI services for assistant and
  job-analysis behavior.

External network behavior must be explicit and mockable. Local validation must
not send messages, mutate remote spreadsheets, or spend provider quota.
