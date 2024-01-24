package main

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/google/btree"
	"github.com/tliron/commonlog"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"

	_ "github.com/tliron/commonlog/simple"
)

const lsName = "wordlsp"

var version = "0.0.1"

type App struct {
	log         commonlog.Logger
	handler     protocol.Handler
	doc         string
	knownWords  *btree.BTreeG[string]
	knownWordsM sync.RWMutex
}

func NewApp() *App {
	a := &App{
		log:        commonlog.GetLogger(lsName),
		knownWords: btree.NewOrderedG[string](2),
	}
	a.handler = protocol.Handler{
		Initialize:             a.initialize,
		Initialized:            a.initialized,
		Shutdown:               a.shutdown,
		SetTrace:               a.setTrace,
		TextDocumentDidOpen:    a.textDocumentDidOpen,
		TextDocumentCompletion: a.textDocumentCompletion,
		TextDocumentDidChange:  a.textDocumentDidChange,
	}
	return a
}

func (a *App) addWords(words []string) {
	a.knownWordsM.Lock()
	defer a.knownWordsM.Unlock()
	for _, word := range words {
		a.knownWords.ReplaceOrInsert(word)
	}
}

func (a *App) completions(prefix string) []string {
	a.knownWordsM.RLock()
	defer a.knownWordsM.RUnlock()
	var results []string
	a.knownWords.AscendGreaterOrEqual(prefix, func(item string) bool {
		if strings.HasPrefix(item, prefix) {
			results = append(results, item)
			return true
		}
		return false
	})
	return results
}

func (a *App) wordStartUntil(p protocol.Position) (int, int) {
	end := p.IndexIn(a.doc)
	startP := p
	start := end
	for startP.Character != 0 {
		possibleStart := startP.IndexIn(a.doc)
		rune, _ := utf8.DecodeLastRuneInString(a.doc[possibleStart:])
		if rune == utf8.RuneError {
			break
		}
		if !unicode.IsLetter(rune) {
			break
		}
		start = possibleStart
		startP.Character--
	}
	return start, end
}

func (a *App) initialize(_ *glsp.Context, _ *protocol.InitializeParams) (any, error) {
	capabilities := a.handler.CreateServerCapabilities()
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    lsName,
			Version: &version,
		},
	}, nil
}

func (a *App) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error {
	return nil
}

func (a *App) shutdown(_ *glsp.Context) error {
	protocol.SetTraceValue(protocol.TraceValueOff)
	return nil
}

func (a *App) setTrace(_ *glsp.Context, params *protocol.SetTraceParams) error {
	protocol.SetTraceValue(params.Value)
	return nil
}

func (a *App) textDocumentDidOpen(_ *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	a.doc = params.TextDocument.Text
	a.addWords(strings.FieldsFunc(a.doc, func(r rune) bool { return !unicode.IsLetter(r) }))
	return nil
}

func (a *App) textDocumentCompletion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	start, end := a.wordStartUntil(params.Position)
	words := a.completions(a.doc[start:end])
	completions := make([]*protocol.CompletionItem, 0, len(words))
	for _, word := range words {
		completions = append(completions, &protocol.CompletionItem{Label: word})
	}
	return completions, nil
}

func (a *App) textDocumentDidChange(_ *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	for _, change := range params.ContentChanges {
		switch c := change.(type) {
		case protocol.TextDocumentContentChangeEvent:
			startIndex, endIndex := c.Range.IndexesIn(a.doc)
			a.doc = a.doc[:startIndex] + c.Text + a.doc[endIndex:]
		case protocol.TextDocumentContentChangeEventWhole:
			a.doc = c.Text
		}
	}
	return nil
}

func main() {
	commonlog.Configure(1, nil)
	s := NewApp()
	server := server.NewServer(&s.handler, lsName, false)
	server.RunStdio()
}
