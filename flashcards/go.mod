module github.com/iadk/k8s-flashcards

go 1.26.6

require (
	github.com/open-spaced-repetition/go-fsrs/v3 v3.3.1
	github.com/yuin/goldmark v1.8.5
	go.abhg.dev/goldmark/mermaid v0.6.0
	go.yaml.in/yaml/v3 v3.0.3
	sigs.k8s.io/yaml v1.6.0
)

require (
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260708182218-49f421fb7959 // indirect
	golang.org/x/tools v0.48.0 // indirect
	golang.org/x/vuln v1.6.0 // indirect
)

tool golang.org/x/vuln/cmd/govulncheck
