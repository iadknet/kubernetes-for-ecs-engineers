// Package chat is the provider-neutral surface behind the drill view's chat
// panel. Everything above it — handlers, templates, process wiring — depends on
// this package and never on a particular backend's flag names, wire format, or
// vendor.
//
// Swapping backends is therefore a new Provider implementation plus one case in
// the process's provider selection; no handler or template moves.
package chat

import "context"

// Turn is one question, carried as structured fields rather than as a
// pre-flattened prompt. Composing them onto a wire is the provider's job: a CLI
// provider concatenates them into one argument, an API provider would place
// them in different message roles entirely.
type Turn struct {
	// Message is what the user typed.
	Message string

	// CardContext describes the card the question is about. The web layer
	// assembles it, because the shape of a card is flashcards domain knowledge
	// and this package deliberately has none.
	CardContext string

	// SystemPrompt carries the tutoring instructions.
	SystemPrompt string

	// Model and Effort are validated against Options before they arrive here.
	// Effort is empty when the provider reports no effort dial.
	Model  string
	Effort string
}

// Options reports what a provider can be asked for. The web layer validates
// requests against it and renders its selectors from it, so no allowlist of
// model or effort names exists above the provider.
type Options struct {
	// Models is every model this provider accepts, in the order the UI offers
	// them. DefaultModel is the one used when a request names none.
	Models       []string
	DefaultModel string

	// Efforts is empty when the provider has no effort dial, and the UI hides
	// the selector rather than offering a control that does nothing.
	Efforts []string
}

// Provider is one conversational backend.
//
// Conversation continuity is the implementation's concern: a CLI provider holds
// a session id, an API provider would hold message history. Callers only know
// that turns accumulate until Reset.
type Provider interface {
	// Send runs one turn, calling emit with each chunk of answer text as it
	// arrives. An error from emit — the client hung up — aborts the turn.
	Send(ctx context.Context, t Turn, emit func(delta string) error) error

	// Reset starts a fresh conversation.
	Reset()

	// Options reports this provider's capabilities.
	Options() Options
}
